"use client";

import type { JSX } from "react";
import { useEffect, useRef, useState } from "react";
import { GiHamburgerMenu } from "react-icons/gi";
import { MdOutlineClose } from "react-icons/md";

export function MenuSelectionClient({ elements }: { elements: JSX.Element[] }): JSX.Element {
    const [isOpen, setIsOpen] = useState(false);
    const menuRef = useRef<HTMLDivElement>(null);

    // Close menu when clicking outside
    useEffect(() => {
        function handleClickOutside(event: MouseEvent) {
            if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
                setIsOpen(false);
            }
        }

        if (isOpen) {
            document.addEventListener("mousedown", handleClickOutside);
        }

        return () => {
            document.removeEventListener("mousedown", handleClickOutside);
        };
    }, [isOpen]);

    return (
        <div className="relative" ref={menuRef}>
            <button
                type="button"
                className="btn btn-primary btn-soft btn-sm border-accent border rounded-xl"
                onClick={() => setIsOpen(!isOpen)}
                aria-label="Toggle menu"
                aria-expanded={isOpen}
            >
                {isOpen ? (
                    <MdOutlineClose className="w-6 h-6 transition-transform duration-300" />
                ) : (
                    <GiHamburgerMenu className="w-6 h-6 transition-transform duration-300" />
                )}
            </button>
            
            {isOpen && (
                <div 
                    role="menu"
                    className="absolute top-full right-0 mt-2 flex flex-col gap-2 p-3 rounded-box backdrop-blur-sm bg-base-100 shadow-lg border border-accent min-w-[200px]"
                    onClick={() => setIsOpen(false)}
                    onKeyDown={(event) => {
                        if (event.key === "Escape") {
                            setIsOpen(false);
                        }
                    }}
                >
                    {elements}
                </div>
            )}
        </div>
    );
}
