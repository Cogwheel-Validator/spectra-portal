"use client";
import { motion } from "motion/react";
import type { JSX } from "react";

interface FlyInButtonProps {
    type: "button" | "submit" | "reset";
    children: JSX.Element;
    className: string;
}

export function FlyInButton({ type, children, className }: FlyInButtonProps): JSX.Element {
    return (
        <motion.button
            initial={{ scale: 0, y: -100 }}
            animate={{ scale: 1, y: 0 }}
            transition={{ duration: 0.5 }}
            className={className}
            type={type}
        >
            {children}
        </motion.button>
    );
}
