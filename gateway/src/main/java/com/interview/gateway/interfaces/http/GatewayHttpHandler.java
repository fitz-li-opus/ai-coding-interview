package com.interview.gateway.interfaces.http;

import com.interview.gateway.application.service.ForwardResponse;
import com.interview.gateway.application.service.GatewayService;
import com.interview.gateway.domain.model.RouteRule;
import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;

import java.io.IOException;
import java.net.http.HttpTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Optional;

public class GatewayHttpHandler implements HttpHandler {
    private static final String API_KEY_HEADER = "X-API-Key";
    private final GatewayService gatewayService;
    private final String expectedApiKey;

    public GatewayHttpHandler(GatewayService gatewayService, String expectedApiKey) {
        this.gatewayService = gatewayService;
        this.expectedApiKey = expectedApiKey;
    }

    @Override
    public void handle(HttpExchange exchange) throws IOException {
        String path = exchange.getRequestURI().getPath();
        if ("/health".equals(path)) {
            writeJson(exchange, 200, "{\"status\":\"UP\",\"service\":\"api-gateway\"}");
            return;
        }

        if (!isAuthorized(exchange)) {
            writeJson(exchange, 401, "{\"error\":\"Unauthorized\"}");
            return;
        }

        Optional<RouteRule> routeRule = gatewayService.resolveRoute(path);
        if (routeRule.isEmpty()) {
            writeJson(exchange, 404, "{\"error\":\"Route not found\"}");
            return;
        }

        byte[] requestBody = exchange.getRequestBody().readAllBytes();
        try {
            ForwardResponse response = gatewayService.forward(
                    routeRule.get(),
                    exchange.getRequestMethod(),
                    path,
                    exchange.getRequestURI().getRawQuery(),
                    exchange.getRequestHeaders(),
                    requestBody
            );
            copyResponse(exchange, response);
        } catch (HttpTimeoutException timeoutException) {
            writeJson(exchange, 504, "{\"error\":\"Upstream timeout\"}");
        } catch (InterruptedException interruptedException) {
            Thread.currentThread().interrupt();
            writeJson(exchange, 500, "{\"error\":\"Request interrupted\"}");
        } catch (IOException ioException) {
            writeJson(exchange, 502, "{\"error\":\"Bad gateway\"}");
        } catch (RuntimeException runtimeException) {
            writeJson(exchange, 500, "{\"error\":\"Gateway internal error\"}");
        }
    }

    private void copyResponse(HttpExchange exchange, ForwardResponse response) throws IOException {
        Headers responseHeaders = exchange.getResponseHeaders();
        response.headers().forEach((header, values) -> {
            if (!"transfer-encoding".equalsIgnoreCase(header) && !"content-length".equalsIgnoreCase(header)) {
                responseHeaders.put(header, List.copyOf(values));
            }
        });
        exchange.sendResponseHeaders(response.statusCode(), response.body().length);
        exchange.getResponseBody().write(response.body());
        exchange.close();
    }

    private void writeJson(HttpExchange exchange, int statusCode, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().set("Content-Type", "application/json; charset=utf-8");
        exchange.sendResponseHeaders(statusCode, bytes.length);
        exchange.getResponseBody().write(bytes);
        exchange.close();
    }

    private boolean isAuthorized(HttpExchange exchange) {
        String providedApiKey = exchange.getRequestHeaders().getFirst(API_KEY_HEADER);
        return expectedApiKey != null
                && !expectedApiKey.isBlank()
                && expectedApiKey.equals(providedApiKey);
    }
}
