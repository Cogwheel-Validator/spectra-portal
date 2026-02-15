import Image from "next/image";
import Link from "next/link";
import type { JSX } from "react";
import type { MenuItem } from "./types";

export default function MenuPC({ menuItems }: { menuItems: MenuItem[] }): JSX.Element {
    return (
        <div className="navbar items-center align-baseline fixed z-20 space-x-4">
            <Image
                src="/logo.png"
                alt="Spectra Logo"
                width={1550}
                height={400}
                className="w-44"
                loading="eager"
            />
            <div className="flex flex-row rounded-box backdrop-blur-sm">
                {menuItems.map((item, index) => (
                    <button
                        type="button"
                        className={`btn btn-primary btn-soft btn-md border-accent border items-center ${
                            index === 0
                                ? "rounded-none rounded-l-box"
                                : index === menuItems.length - 1
                                  ? "rounded-none rounded-r-box"
                                  : "rounded-none"
                        }`}
                        key={item.label}
                    >
                        {item.icon && <span>{item.icon as React.ReactNode}</span>}
                        <Link
                            href={item.href}
                            target={item.newTab ? "_blank" : undefined}
                            rel={item.newTab ? "noopener noreferrer" : undefined}
                        >
                            {item.label}
                        </Link>
                    </button>
                ))}
            </div>
        </div>
    );
}
