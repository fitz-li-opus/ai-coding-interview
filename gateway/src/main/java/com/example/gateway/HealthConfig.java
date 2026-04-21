package com.example.gateway;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.actuate.health.ReactiveHealthIndicator;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class HealthConfig {

    @Value("${USER_SERVICE_URL:http://localhost:8081}")
    private String userServiceUrl;

    @Value("${ORDER_SERVICE_URL:http://localhost:8082}")
    private String orderServiceUrl;

    @Value("${PRODUCT_SERVICE_URL:http://localhost:8083}")
    private String productServiceUrl;

    @Bean
    public ReactiveHealthIndicator userService() {
        return new BackendHealthIndicator("user-service", userServiceUrl);
    }

    @Bean
    public ReactiveHealthIndicator orderService() {
        return new BackendHealthIndicator("order-service", orderServiceUrl);
    }

    @Bean
    public ReactiveHealthIndicator productService() {
        return new BackendHealthIndicator("product-service", productServiceUrl);
    }
}
