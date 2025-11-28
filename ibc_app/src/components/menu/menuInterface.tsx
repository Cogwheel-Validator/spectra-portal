import type { JSX } from "react";
import MenuPC from "./menuPC";
import MenuPhone from "./menuPhone";
import type { MenuItem } from "./types";

export default function MenuInterface(): JSX.Element {
    const menuItems: MenuItem[] = [
        { label: "IBC Send", href: "/ibc" },
        { label: "Chain Status", href: "/status" },
        { label: "About Spectra IBC", href: "/about" },
        { label: "Docs", href: "https://docs.cogwheel.zone/spectra-ibc", newTab: true },
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