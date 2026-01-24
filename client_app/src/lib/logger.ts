import pino from "pino";
import pretty from "pino-pretty";

const stream = pretty({
    colorize: true,
});

const logger = pino(
    {
        name: "general-portal-logger",
        level: process.env.NODE_ENV === "development" ? "debug" : "info",
    },
    stream,
);

export default logger;
