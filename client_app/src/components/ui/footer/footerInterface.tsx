import type { JSX } from "react";
import FooterPC from "./footerPC";
import FooterPhone from "./footerPhone";

export default function FooterInterface(): JSX.Element {
    return (
        <>
            <FooterPhone className="block lg:hidden" />
            <FooterPC className="hidden lg:block" />
        </>
    );
}
