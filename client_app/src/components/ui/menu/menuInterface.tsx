import type { JSX } from "react";
import { BiTransfer } from "react-icons/bi";
import { FaInfo } from "react-icons/fa";
import { SiGoogledocs } from "react-icons/si";
import MenuPC from "./menuPC";
import MenuPhone from "./menuPhone";
import type { MenuItem } from "./types";

export default function MenuInterface(): JSX.Element {
    const menuItems: MenuItem[] = [
        { label: "Transfer", href: "/transfer", icon: <BiTransfer className="size-6" /> },
        { label: "About Portal App", href: "/about", icon: <FaInfo className="size-6" /> },
        {
            label: "Docs",
            href: "https://docs.cogwheel.zone/spectra-portal",
            newTab: true,
            icon: <SiGoogledocs className="size-6" />,
        },
    ];

    return (
        <>
            {/* Show phone menu on small and medium screens */}
            <div className="block lg:hidden">
                <MenuPhone menuItems={menuItems} />
            </div>
            {/* Show PC menu on larger screens */}
            <div className="hidden lg:block">
                <MenuPC menuItems={menuItems} />
            </div>
        </>
    );
}
