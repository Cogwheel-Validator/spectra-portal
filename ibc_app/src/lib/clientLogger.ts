/**
 * Simple client-side logger that works in browser environments.
 * Falls back to console.* methods with optional namespacing.
 */

type LogLevel = "trace" | "debug" | "info" | "warn" | "error" | "fatal";

interface ClientLogger {
    trace: (...args: unknown[]) => void;
    debug: (...args: unknown[]) => void;
    info: (...args: unknown[]) => void;
    warn: (...args: unknown[]) => void;
    error: (...args: unknown[]) => void;
    fatal: (...args: unknown[]) => void;
}

const LOG_LEVELS: Record<LogLevel, number> = {
    trace: 10,
    debug: 20,
    info: 30,
    warn: 40,
    error: 50,
    fatal: 60,
};

function createClientLogger(name: string, minLevel: LogLevel = "info"): ClientLogger {
    const minLevelNum = LOG_LEVELS[minLevel];
    const prefix = `[${name}]`;

    const shouldLog = (level: LogLevel): boolean => {
        return LOG_LEVELS[level] >= minLevelNum;
    };

    return {
        trace: (...args: unknown[]) => {
            if (shouldLog("trace")) console.debug(prefix, ...args);
        },
        debug: (...args: unknown[]) => {
            if (shouldLog("debug")) console.debug(prefix, ...args);
        },
        info: (...args: unknown[]) => {
            if (shouldLog("info")) console.info(prefix, ...args);
        },
        warn: (...args: unknown[]) => {
            if (shouldLog("warn")) console.warn(prefix, ...args);
        },
        error: (...args: unknown[]) => {
            if (shouldLog("error")) console.error(prefix, ...args);
        },
        fatal: (...args: unknown[]) => {
            if (shouldLog("fatal")) console.error(prefix, "[FATAL]", ...args);
        },
    };
}

// Default client logger for the app
const clientLogger = createClientLogger(
    "spectra-ibc",
    (process.env.NODE_ENV === "development" ? "debug" : "info") as LogLevel,
);

export { createClientLogger, type ClientLogger };
export default clientLogger;
