"use client";

import { useEffect, useRef, useState } from "react";

/**
 * Debounces a value by the specified delay
 * @param value - The value to debounce
 * @param delay - The delay in milliseconds (default: 1000ms)
 * @returns The debounced value
 * 
 * Usage in this case can be when inserting into the amount section so we make query the the Pathfinder
 * after the user has stopped typing for a certain amount of time.
 */
export function useDebounce<T>(value: T, delay: number = 1000): T {
    const [debouncedValue, setDebouncedValue] = useState<T>(value);

    useEffect(() => {
        const timer = setTimeout(() => {
            setDebouncedValue(value);
        }, delay);

        return () => {
            clearTimeout(timer);
        };
    }, [value, delay]);

    return debouncedValue;
}

/**
 * Returns a debounced callback function
 * @param callback - The callback function to debounce
 * @param delay - The delay in milliseconds (default: 1000ms)
 * @returns A debounced version of the callback
 * 
 * Usage in this case can be when inserting into the amount section so we make query the the Pathfinder
 * after the user has stopped typing for a certain amount of time. Just that this function uses 
 * a ref to the callback function and not the callback itself so we can avoid recreating the function on each render.
 */
export function useDebouncedCallback<T extends (...args: Parameters<T>) => ReturnType<T>>(
    callback: T,
    delay: number = 1000,
): (...args: Parameters<T>) => void {
    const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const callbackRef = useRef(callback);

    // Update callback ref when callback changes
    useEffect(() => {
        callbackRef.current = callback;
    }, [callback]);

    // Cleanup on unmount
    useEffect(() => {
        return () => {
            if (timeoutRef.current) {
                clearTimeout(timeoutRef.current);
            }
        };
    }, []);

    return (...args: Parameters<T>) => {
        if (timeoutRef.current) {
            clearTimeout(timeoutRef.current);
        }

        timeoutRef.current = setTimeout(() => {
            callbackRef.current(...args);
        }, delay);
    };
}

/**
 * Returns whether the debounced value is currently pending (value changed but debounce hasn't fired)
 * @param value - The current value
 * @param debouncedValue - The debounced value
 * @returns True if the value is pending (different from debounced value)
 */
export function useIsPending<T>(value: T, debouncedValue: T): boolean {
    return value !== debouncedValue;
}
