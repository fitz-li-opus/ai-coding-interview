package com.interview.gateway.fallback;

import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;

/**
 * 统一降级响应端点
 *
 * <p>可作为路由 fallback 目标（uri: forward:/fallback）返回统一降级 JSON，
 * 与 Sentinel block handler 保持一致的响应结构，方便客户端统一处理。
 */
@RestController
@RequestMapping("/fallback")
public class FallbackController {

    @GetMapping
    @ResponseStatus(HttpStatus.SERVICE_UNAVAILABLE)
    public Map<String, Object> fallback() {
        return Map.of(
                "code", 503,
                "message", "服务暂不可用，已降级兜底，请稍后重试",
                "service", "api-gateway"
        );
    }
}
