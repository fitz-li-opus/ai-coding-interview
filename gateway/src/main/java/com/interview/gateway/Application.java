package com.interview.gateway;

import com.interview.gateway.application.service.GatewayService;
import com.interview.gateway.domain.model.RouteRule;
import com.interview.gateway.domain.service.RouteResolver;
import com.interview.gateway.infrastructure.http.JavaHttpForwarder;
import com.interview.gateway.interfaces.http.GatewayHttpHandler;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.http.HttpClient;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.Executors;

public class Application {
    public static void main(String[] args) throws IOException {
        int port = Integer.parseInt(System.getenv().getOrDefault("GATEWAY_PORT", "8080"));
        String gatewayApiKey = System.getenv().getOrDefault("GATEWAY_API_KEY", "interview-key");

        List<RouteRule> routeRules = List.of(
                new RouteRule("/api/users", URI.create(System.getenv().getOrDefault("USER_SERVICE_URL", "http://localhost:8081"))),
                new RouteRule("/api/orders", URI.create(System.getenv().getOrDefault("ORDER_SERVICE_URL", "http://localhost:8082"))),
                new RouteRule("/api/products", URI.create(System.getenv().getOrDefault("PRODUCT_SERVICE_URL", "http://localhost:8083")))
        );

        RouteResolver routeResolver = new RouteResolver(routeRules);
        JavaHttpForwarder javaHttpForwarder = new JavaHttpForwarder(
                HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(3)).build(),
                Duration.ofSeconds(10)
        );
        GatewayService gatewayService = new GatewayService(routeResolver, javaHttpForwarder);

        HttpServer httpServer = HttpServer.create(new InetSocketAddress(port), 0);
        httpServer.createContext("/", new GatewayHttpHandler(gatewayService, gatewayApiKey));
        httpServer.setExecutor(Executors.newVirtualThreadPerTaskExecutor());
        httpServer.start();

        // TODO: Add structured logging and request-id tracing.
        // TODO: Replace static API key with real authn/authz.
        // TODO: Add metrics and distributed tracing.
        System.out.println("API Gateway listening on :" + port);
    }
}
