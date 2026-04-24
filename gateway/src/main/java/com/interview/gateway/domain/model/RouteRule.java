package com.interview.gateway.domain.model;

import java.net.URI;

public record RouteRule(String prefix, URI upstreamBaseUri) {
}
