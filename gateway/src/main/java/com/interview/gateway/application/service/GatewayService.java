package com.interview.gateway.application.service;

import com.interview.gateway.domain.model.RouteRule;
import com.interview.gateway.domain.service.RouteResolver;

import java.io.IOException;
import java.net.http.HttpTimeoutException;
import java.util.List;
import java.util.Map;
import java.util.Optional;

public class GatewayService {
    private final RouteResolver routeResolver;
    private final HttpForwarder httpForwarder;

    public GatewayService(RouteResolver routeResolver, HttpForwarder httpForwarder) {
        this.routeResolver = routeResolver;
        this.httpForwarder = httpForwarder;
    }

    public Optional<RouteRule> resolveRoute(String path) {
        return routeResolver.resolve(path);
    }

    public ForwardResponse forward(RouteRule routeRule,
                                   String method,
                                   String path,
                                   String rawQuery,
                                   Map<String, List<String>> requestHeaders,
                                   byte[] requestBody) throws IOException, InterruptedException, HttpTimeoutException {
        return httpForwarder.forward(routeRule, method, path, rawQuery, requestHeaders, requestBody);
    }
}
