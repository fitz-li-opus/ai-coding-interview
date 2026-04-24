package com.interview.gateway.domain.service;

import com.interview.gateway.domain.model.RouteRule;

import java.util.Comparator;
import java.util.List;
import java.util.Optional;

public class RouteResolver {
    private final List<RouteRule> routeRules;

    public RouteResolver(List<RouteRule> routeRules) {
        this.routeRules = routeRules.stream()
                .sorted(Comparator.comparingInt((RouteRule rule) -> rule.prefix().length()).reversed())
                .toList();
    }

    public Optional<RouteRule> resolve(String path) {
        return routeRules.stream()
                .filter(rule -> path.startsWith(rule.prefix()))
                .findFirst();
    }
}
