import Image from "next/image";
import Link from "next/link";
import type { JSX } from "react";
import type { MenuItem } from "./types";

export default function MenuPhone({ menuItems }: { menuItems: MenuItem[] }): JSX.Element {
    const maxElements: number = 3;
    const menu: MenuItem[] = [];
    const overflowItems: MenuItem[] = [];
    for (let i = 0; i < menuItems.length; i++) {
        if (i < maxElements) {
            menu.push(menuItems[i]);
        } else {
            overflowItems.push(menuItems[i]);
        }
    }
    return (
        <div className="navbar items-center align-baseline fixed z-20 space-x-4">
            <Image
                src="/logo.png"
                alt="Spectra Logo"
                width={521}
                height={126}
                className="w-20"
                loading="eager"
            />
            <div className="flex flex-row rounded-box backdrop-blur-sm">
                {menu.map((item, index) => (
                    <button
                        type="button"
                        className={`btn btn-primary btn-soft btn-sm border-accent border ${
                            index === 0 ? "rounded-none rounded-l-box" : "rounded-none"
                        }`}
                        key={item.label}
                    >
                        <Link
                            href={item.href}
                            target={item.newTab ? "_blank" : undefined}
                            rel={item.newTab ? "noopener noreferrer" : undefined}
                        >
                            {item.label}
                        </Link>
                    </button>
                ))}
                {overflowItems.length > 0 && (
                    <ul className="menu btn btn-primary btn-soft btn-sm border-accent border rounded-r-box">
                        ...
                        {overflowItems.map((item) => (
                            <li key={item.label}>
                                <Link
                                    href={item.href}
                                    target={item.newTab ? "_blank" : undefined}
                                    rel={item.newTab ? "noopener noreferrer" : undefined}
                                >
                                    {item.label}
                                </Link>
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </div>
    );
}
