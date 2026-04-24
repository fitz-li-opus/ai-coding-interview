package com.interview.gateway.application.service;

import com.interview.gateway.domain.model.RouteRule;

import java.io.IOException;
import java.net.http.HttpTimeoutException;
import java.util.List;
import java.util.Map;

public interface HttpForwarder {
    ForwardResponse forward(RouteRule routeRule,
                            String method,
                            String path,
                            String rawQuery,
                            Map<String, List<String>> requestHeaders,
                            byte[] requestBody) throws IOException, InterruptedException, HttpTimeoutException;
}
