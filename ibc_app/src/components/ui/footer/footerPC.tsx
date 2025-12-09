import Image from "next/image";
import Link from "next/link";
import type { JSX } from "react";

export default function FooterPC(): JSX.Element {
    const year = new Date().getFullYear();
    return (
        <footer className="flex flex-col p-4 bg-base-300 min-h-[20dvh] text-base-content">
            <div className="flex flex-row items-center justify-between">
                {/*Powered by Cogwheel Logo*/}
                <div>
                <h3>Developed by:</h3>
                <Link href="https://cogwheel.zone" target="_blank" rel="noopener noreferrer" className="hover:opacity-80 transition-opacity duration-300">
                <Image src="/cogwheel-logo.png" alt="Cogwheel Logo" width={521} height={126} className="w-40" loading="eager" />
                </Link>
                </div>
                <div>
                {/*Spectra Solver RPC Logo*/}
                <h3>Powered By:</h3>
                <Link href="https://docs.cogwheel.zone/spectra-ibc" target="_blank" rel="noopener noreferrer" className="hover:opacity-80 transition-opacity duration-300">
                {/*Use spectra logo for now but another should take place */}
                <Image src="/logo.png" alt="Spectra Solver RPC Logo" width={521} height={126} className="w-40" loading="eager" />
                </Link>
                </div>
            </div>
                <p className="text-sm text-center">© {year} Cogwheel Validator. All rights reserved.</p>
        </footer>
    );
}