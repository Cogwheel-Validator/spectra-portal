import { type NextRequest, NextResponse } from "next/server";
import pino from "pino";
import pretty from "pino-pretty";

const stream = pretty({
    colorize: true,
});

const logger = pino(
    {
        name: "nextjs-request-logger",
        level: process.env.NODE_ENV === "development" ? "debug" : "info",
    },
    stream,
);

const LOGGABLE_PATHS = ["/api/", "/transfer", "/"];

const IGNORE_PATHS = ["/_next/", ".svg", ".js", ".css", ".woff", ".png", ".jpg", ".ico"];

function shouldLog(url: string): boolean {
    if (IGNORE_PATHS.some((path) => url.includes(path))) {
        return false;
    }

    if (LOGGABLE_PATHS.length > 0) {
        return LOGGABLE_PATHS.some((path) => url.includes(path));
    }

    return true;
}

function formatLog(request: NextRequest) {
    const url = new URL(request.url);

    return {
        method: request.method,
        path: url.pathname + url.search,
        userAgent: request.headers.get("user-agent")?.substring(0, 50),
        referer: request.headers.get("referer"),
    };
}

export async function proxy(request: NextRequest) {
    if (shouldLog(request.url)) {
        logger.info(formatLog(request));
    }
    return NextResponse.next();
}
