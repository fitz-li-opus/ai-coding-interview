package com.example.gateway;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.reactive.server.WebTestClient;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class CorsTest {

    @Autowired
    private WebTestClient webTestClient;

    @Test
    void preflightRequestReturns200() {
        webTestClient.options()
                .uri("/api/users/")
                .header("Origin", "http://localhost:3000")
                .header("Access-Control-Request-Method", "GET")
                .exchange()
                .expectStatus().isOk();
    }

    @Test
    void corsHeadersPresentOnActualRequest() {
        webTestClient.get()
                .uri("/api/users/")
                .header("Origin", "http://localhost:3000")
                .exchange()
                .expectHeader().exists("Access-Control-Allow-Origin");
    }
}
