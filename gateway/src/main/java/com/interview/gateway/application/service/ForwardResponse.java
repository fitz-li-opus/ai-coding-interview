package com.interview.gateway.application.service;

import java.util.List;
import java.util.Map;

public record ForwardResponse(int statusCode, Map<String, List<String>> headers, byte[] body) {
}
