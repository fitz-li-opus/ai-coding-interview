package com.example.gateway;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.reactive.server.WebTestClient;

@SpringBootTest(webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT)
class ErrorHandlingTest {

    @Autowired
    private WebTestClient webTestClient;

    @Test
    void unknownRouteReturns404WithJsonBody() {
        webTestClient.get()
                .uri("/not-exist")
                .exchange()
                .expectStatus().isNotFound()
                .expectHeader().contentType("application/json")
                .expectBody()
                .jsonPath("$.status").isEqualTo(404)
                .jsonPath("$.error").isEqualTo("Not Found")
                .jsonPath("$.path").isEqualTo("/not-exist");
    }

    @Test
    void errorResponseHasNoSpringTimestampField() {
        webTestClient.get()
                .uri("/not-exist")
                .exchange()
                .expectBody()
                .jsonPath("$.timestamp").doesNotExist();
    }
}
