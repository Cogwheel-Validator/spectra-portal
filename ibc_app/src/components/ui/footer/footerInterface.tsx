import type { JSX } from "react";
import FooterPC from "./footerPC";
import FooterPhone from "./footerPhone";

export default function FooterInterface(): JSX.Element {
    return (
        <>
            <div className="block lg:hidden">
                <FooterPhone />
            </div>
            <div className="hidden lg:block">
                <FooterPC />
            </div>
        </>
    );
}
