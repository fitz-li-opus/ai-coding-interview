package com.example.gateway;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.reactive.server.WebTestClient;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class GatewayRoutingTest {

    @Autowired
    private WebTestClient webTestClient;

    @Test
    void usersRouteReturns200() {
        webTestClient.get()
                .uri("/api/users/")
                .exchange()
                .expectStatus().isOk();
    }

    @Test
    void ordersRouteReturns200() {
        webTestClient.get()
                .uri("/api/orders/")
                .exchange()
                .expectStatus().isOk();
    }

    @Test
    void productsRouteReturns200() {
        webTestClient.get()
                .uri("/api/products/")
                .exchange()
                .expectStatus().isOk();
    }
}
