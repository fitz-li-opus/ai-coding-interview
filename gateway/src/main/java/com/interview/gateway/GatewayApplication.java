package com.interview.gateway;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * API 网关启动类
 *
 * <p>技术栈：JDK 17 + Spring Boot 3.2 + Spring Cloud 2023 + Spring Cloud Alibaba
 * （Nacos 注册中心 / 配置中心 + Sentinel 熔断降级）
 */
@SpringBootApplication
public class GatewayApplication {

    public static void main(String[] args) {
        SpringApplication.run(GatewayApplication.class, args);
    }
}
