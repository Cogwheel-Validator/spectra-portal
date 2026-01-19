"use client";

/**
 * Simple client-side logger that works in browser environments.
 * Falls back to console.* methods with optional namespacing.
 * All values are converted to strings for security - no object references are preserved.
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

/**
 * Safely converts any value to a string representation.
 * This ensures no object references or memory access is possible from logs.
 * Once converted to string, the original object cannot be recovered.
 */
function safeStringify(...args: unknown[]): string {
    const stringified = args.map((arg) => {
        // Handle primitives
        if (arg === null) return "null";
        if (arg === undefined) return "undefined";
        if (typeof arg === "string") return arg;
        if (typeof arg === "number" || typeof arg === "boolean" || typeof arg === "bigint") {
            return String(arg);
        }
        
        // Handle objects, arrays, functions, etc. - convert to JSON string
        // This ensures no references are preserved
        try {
            return JSON.stringify(arg, (_key, value) => {
                // Convert functions, symbols, undefined to strings
                if (typeof value === "function") return `[Function: ${value.name || "anonymous"}]`;
                if (typeof value === "symbol") return String(value);
                if (typeof value === "undefined") return "undefined";
                // For objects with circular refs, JSON.stringify will throw, caught below
                return value;
            }, 2);
        } catch (error) {
            // Handle circular references or other JSON.stringify errors
            if (error instanceof Error && error.message.includes("circular")) {
                return "[Circular Reference]";
            }
            // Fallback: use Object.prototype.toString for anything else
            try {
                return String(arg);
            } catch {
                return "[Unable to stringify]";
            }
        }
    });
    
    // Join with spaces (similar to util.format behavior)
    return stringified.join(" ");
}

function createClientLogger(name: string, minLevel: LogLevel = "info"): ClientLogger {
    const minLevelNum = LOG_LEVELS[minLevel];
    const prefix = `[${name}]`;

    const shouldLog = (level: LogLevel): boolean => {
        return LOG_LEVELS[level] >= minLevelNum;
    };

    // All values are converted to strings - no object references preserved
    // This ensures logs cannot be used to access memory or sensitive data
    return {
        trace: (...args: unknown[]) => {
            if (shouldLog("trace")) console.debug(prefix, safeStringify(...args));
        },
        debug: (...args: unknown[]) => {
            if (shouldLog("debug")) console.debug(prefix, safeStringify(...args));
        },
        info: (...args: unknown[]) => {
            if (shouldLog("info")) console.info(prefix, safeStringify(...args));
        },
        warn: (...args: unknown[]) => {
            if (shouldLog("warn")) console.warn(prefix, safeStringify(...args));
        },
        error: (...args: unknown[]) => {
            if (shouldLog("error")) console.error(prefix, safeStringify(...args));
        },
        fatal: (...args: unknown[]) => {
            if (shouldLog("fatal")) console.error(prefix, "[FATAL]", safeStringify(...args));
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
