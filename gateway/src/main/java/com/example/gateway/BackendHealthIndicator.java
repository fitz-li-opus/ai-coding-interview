package com.example.gateway;

import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.ReactiveHealthIndicator;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;

public class BackendHealthIndicator implements ReactiveHealthIndicator {

    private final WebClient webClient;
    private final String name;
    private final String url;

    public BackendHealthIndicator(String name, String url) {
        this.name = name;
        this.url = url;
        this.webClient = WebClient.create();
    }

    @Override
    public Mono<Health> health() {
        return webClient.get()
                .uri(url + "/health")
                .retrieve()
                .toBodilessEntity()
                .map(response -> Health.up().withDetail("url", url).build())
                .onErrorResume(ex -> Mono.just(
                        Health.down().withDetail("url", url).withDetail("error", ex.getMessage()).build()
                ));
    }
}
