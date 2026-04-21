package com.example.gateway;

import org.springframework.boot.actuate.health.ReactiveHealthIndicator;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class HealthConfig {

    @Bean
    public ReactiveHealthIndicator userService() {
        return new BackendHealthIndicator("user-service", "http://localhost:8081");
    }

    @Bean
    public ReactiveHealthIndicator orderService() {
        return new BackendHealthIndicator("order-service", "http://localhost:8082");
    }

    @Bean
    public ReactiveHealthIndicator productService() {
        return new BackendHealthIndicator("product-service", "http://localhost:8083");
    }
}
