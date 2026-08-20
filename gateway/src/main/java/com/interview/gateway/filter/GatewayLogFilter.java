package com.interview.gateway.filter;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.cloud.gateway.filter.GatewayFilterChain;
import org.springframework.cloud.gateway.filter.GlobalFilter;
import org.springframework.core.Ordered;
import org.springframework.stereotype.Component;
import org.springframework.web.server.ServerWebExchange;
import reactor.core.publisher.Mono;

/**
 * 全局访问日志过滤器
 *
 * <p>打印每个请求的方法/路径/耗时，是网关扩展点的示例：
 * 可在此追加鉴权、限流、链路追踪（traceId）等通用逻辑。
 */
@Component
public class GatewayLogFilter implements GlobalFilter, Ordered {

    private static final Logger log = LoggerFactory.getLogger(GatewayLogFilter.class);
    private static final String START_TIME = "gateway.startTime";

    @Override
    public Mono<Void> filter(ServerWebExchange exchange, GatewayFilterChain chain) {
        exchange.getAttributes().put(START_TIME, System.currentTimeMillis());
        log.info("[Gateway] {} {}", exchange.getRequest().getMethod(), exchange.getRequest().getURI());
        return chain.filter(exchange).then(Mono.fromRunnable(() -> {
            long cost = System.currentTimeMillis() - (Long) exchange.getAttributes().get(START_TIME);
            int status = exchange.getResponse().getStatusCode() == null ? 0
                    : exchange.getResponse().getStatusCode().value();
            log.info("[Gateway] {} {} -> {} cost={}ms", exchange.getRequest().getMethod(),
                    exchange.getRequest().getURI().getPath(), status, cost);
        }));
    }

    @Override
    public int getOrder() {
        // 排在 Sentinel 过滤器（-1）之后
        return 0;
    }
}
