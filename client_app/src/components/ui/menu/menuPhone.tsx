import Image from "next/image";
import Link from "next/link";
import type { JSX } from "react";
import { MenuSelectionClient } from "@/components/ui/menu/menuPhoneClient";
import type { MenuItem } from "./types";

export default function MenuPhone({ menuItems }: { menuItems: MenuItem[] }): JSX.Element {
    const menuElements: JSX.Element[] = generateJSXMenuSelection(menuItems);

    return (
        <div className="navbar items-center justify-between align-baseline fixed z-20 space-x-4">
            <Image
                src="/logo.png"
                alt="Spectra Logo"
                width={1550}
                height={400}
                className="w-28"
                loading="eager"
            />
            <MenuSelectionClient elements={menuElements} />
        </div>
    );
}

function generateJSXMenuSelection(items: MenuItem[]): JSX.Element[] {
    return items.map((item, index) => (
        <Link
            key={`${item.label}-${index}`}
            href={item.href}
            target={item.newTab ? "_blank" : undefined}
            rel={item.newTab ? "noopener noreferrer" : undefined}
            className="btn btn-primary btn-soft btn-sm border-accent border rounded-xl w-full items-center"
            prefetch={item.prefetch ?? true}
        >
            {item.icon && <span>{item.icon as React.ReactNode}</span>}
            {item.label}
        </Link>
    ));
}
