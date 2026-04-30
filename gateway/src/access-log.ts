export type AccessLogEntry = {
  durationMs: number;
  method: string;
  path: string;
  requestId: string;
  status: number;
};

export type Logger = (entry: AccessLogEntry) => void;

export const createAccessLogEntry = (
  request: Request,
  response: Response,
  startedAt: number,
): AccessLogEntry => {
  const url = new URL(request.url);

  return {
    durationMs: Math.round(performance.now() - startedAt),
    method: request.method,
    path: url.pathname,
    requestId: response.headers.get("x-request-id") ?? request.headers.get("x-request-id") ?? "",
    status: response.status,
  };
};

export const stdoutLogger: Logger = (entry) =>
  console.log(
    JSON.stringify({
      level: "info",
      message: "request",
      ...entry,
    }),
  );
