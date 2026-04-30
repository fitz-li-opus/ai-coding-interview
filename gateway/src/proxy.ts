import type { ServiceRoute } from "./config.ts";

export type Fetcher = (input: URL | Request | string, init?: RequestInit) => Promise<Response>;

export type ProxyOptions = {
  requestTimeoutMs: number;
  services: ServiceRoute[];
  fetcher?: Fetcher;
};

const HOP_BY_HOP_HEADERS = [
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "host",
  "content-length",
];

export const findRoute = (pathname: string, services: ServiceRoute[]) =>
  services.find(
    (service) => pathname === service.prefix || pathname.startsWith(`${service.prefix}/`),
  );

const buildTargetUrl = (requestUrl: string, service: ServiceRoute) => {
  const incomingUrl = new URL(requestUrl);
  const targetUrl = new URL(service.upstreamUrl);

  targetUrl.pathname = incomingUrl.pathname;
  targetUrl.search = incomingUrl.search;

  return targetUrl;
};

const buildForwardHeaders = (request: Request) => {
  const headers = new Headers(request.headers);

  for (const header of HOP_BY_HOP_HEADERS) headers.delete(header);

  const requestId = headers.get("x-request-id") ?? crypto.randomUUID();
  const incomingUrl = new URL(request.url);

  headers.set("x-request-id", requestId);
  headers.set("x-forwarded-host", request.headers.get("host") ?? "");
  headers.set("x-forwarded-proto", incomingUrl.protocol.replace(":", ""));

  return headers;
};

const hasRequestBody = (method: string) => method !== "GET" && method !== "HEAD";

const proxyErrorResponse = (error: unknown) => {
  if (error instanceof DOMException && error.name === "AbortError") {
    return Response.json(
      { error: "gateway_timeout", message: "Upstream request timed out" },
      { status: 504 },
    );
  }

  return Response.json(
    { error: "bad_gateway", message: "Upstream request failed" },
    { status: 502 },
  );
};

export const proxyRequest = async (request: Request, options: ProxyOptions): Promise<Response> => {
  const route = findRoute(new URL(request.url).pathname, options.services);

  if (!route) {
    return Response.json(
      { error: "not_found", message: "No route configured for path" },
      { status: 404 },
    );
  }

  const abortController = new AbortController();
  const timeout = setTimeout(() => abortController.abort(), options.requestTimeoutMs);

  try {
    const upstreamResponse = await (options.fetcher ?? fetch)(buildTargetUrl(request.url, route), {
      body: hasRequestBody(request.method) ? request.body : undefined,
      headers: buildForwardHeaders(request),
      method: request.method,
      redirect: "manual",
      signal: abortController.signal,
    });

    return new Response(upstreamResponse.body, {
      headers: upstreamResponse.headers,
      status: upstreamResponse.status,
      statusText: upstreamResponse.statusText,
    });
  } catch (error) {
    return proxyErrorResponse(error);
  } finally {
    clearTimeout(timeout);
  }
};
