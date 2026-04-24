package com.interview.gateway.infrastructure.http;

import com.interview.gateway.application.service.ForwardResponse;
import com.interview.gateway.application.service.HttpForwarder;
import com.interview.gateway.domain.model.RouteRule;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.Map;

public class JavaHttpForwarder implements HttpForwarder {
    private final HttpClient httpClient;
    private final Duration upstreamTimeout;

    public JavaHttpForwarder(HttpClient httpClient, Duration upstreamTimeout) {
        this.httpClient = httpClient;
        this.upstreamTimeout = upstreamTimeout;
    }

    @Override
    public ForwardResponse forward(RouteRule routeRule,
                                   String method,
                                   String path,
                                   String rawQuery,
                                   Map<String, List<String>> requestHeaders,
                                   byte[] requestBody) throws IOException, InterruptedException {
        URI upstreamUri = createUpstreamUri(routeRule.upstreamBaseUri(), path, rawQuery);

        HttpRequest.Builder requestBuilder = HttpRequest.newBuilder(upstreamUri)
                .timeout(upstreamTimeout)
                .method(method, buildBodyPublisher(requestBody));

        requestHeaders.forEach((header, values) -> {
            if (isForwardableHeader(header)) {
                for (String value : values) {
                    requestBuilder.header(header, value);
                }
            }
        });

        HttpResponse<byte[]> response = httpClient.send(requestBuilder.build(), HttpResponse.BodyHandlers.ofByteArray());
        return new ForwardResponse(response.statusCode(), response.headers().map(), response.body());
    }

    private URI createUpstreamUri(URI upstreamBaseUri, String path, String rawQuery) {
        String queryPart = (rawQuery == null || rawQuery.isBlank()) ? "" : "?" + rawQuery;
        return URI.create(upstreamBaseUri + path + queryPart);
    }

    private HttpRequest.BodyPublisher buildBodyPublisher(byte[] requestBody) {
        if (requestBody == null || requestBody.length == 0) {
            return HttpRequest.BodyPublishers.noBody();
        }
        return HttpRequest.BodyPublishers.ofByteArray(requestBody);
    }

    private boolean isForwardableHeader(String header) {
        if (header == null || header.isBlank()) {
            return false;
        }
        String normalized = header.trim().toLowerCase();
        return !"host".equals(normalized)
                && !"content-length".equals(normalized)
                && !"connection".equals(normalized)
                && !"upgrade".equals(normalized)
                && !"expect".equals(normalized)
                && !"transfer-encoding".equals(normalized);
    }
}
