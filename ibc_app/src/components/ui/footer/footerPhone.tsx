import Image from "next/image";
import Link from "next/link";
import type { JSX } from "react";

// this function is just a copy so I can test the pc version
export default function FooterPhone(): JSX.Element {
    return (
        <footer className="footer footer-center p-4 bg-base-300 text-base-content">
            <div className="flex flex-col items-center justify-between">
                {/*Powered by Cogwheel Logo*/}
                <Link href="https://cogwheel.zone" target="_blank" rel="noopener noreferrer" className="hover:opacity-80 transition-opacity duration-300">
                <Image src="/cogwheel_logo.png" alt="Cogwheel Logo" width={521} height={126} className="w-40" loading="eager" />
                </Link>
                {/*Spectra Solver RPC Logo*/}
                <Link href="https://docs.cogwheel.zone/spectra-ibc" target="_blank" rel="noopener noreferrer" className="hover:opacity-80 transition-opacity duration-300">
                {/*Use spectra logo for now but another should take place */}
                <Image src="/spectra_logo.png" alt="Spectra Solver RPC Logo" width={521} height={126} className="w-40" loading="eager" />
                </Link>
            </div>
                <p className="text-sm">© 2025 Spectra IBC Hub. All rights reserved.</p>
        </footer>
    );
}