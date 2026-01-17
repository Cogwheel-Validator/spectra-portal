import Image from "next/image";
import Link from "next/link";
import type { JSX } from "react";
import { FaGlobe, FaSquareGithub, FaSquareXTwitter } from "react-icons/fa6";

interface footerPCProps {
    className?: string;
}

export default function FooterPC({ className }: footerPCProps): JSX.Element {
    const year = new Date().getFullYear();
    return (
        <footer className={`bg-transparent absolute bottom-0 w-full z-20 p-4 mt-auto ${className}`}>
            <div className="flex flex-row items-center justify-between">
                {/*Powered by Cogwheel Logo*/}
                <div>
                    <h4 className="text-base-content">Developed by:</h4>
                    <Link
                        href="https://cogwheel.zone"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="hover:opacity-80 transition-opacity duration-300"
                    >
                        <Image
                            src="/cogwheel-logo.png"
                            alt="Cogwheel Logo"
                            width={521}
                            height={126}
                            className="w-40 xl:w-44 2xl:w-48"
                            loading="eager"
                        />
                    </Link>
                </div>
                {/**Social icons and links */}
                <div className="flex flex-row space-x-4">
                    <Link
                        href="https://x.com/cogwheel_val"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="tooltip"
                        data-tip="Follow us on X"
                    >
                        <FaSquareXTwitter className="size-10 text-base-content" />
                    </Link>
                    <Link
                        href="https://github.com/Cogwheel-Validator/spectra-ibc-hub"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="tooltip"
                        data-tip="IBC Hub Repo"
                    >
                        <FaSquareGithub className="size-10 text-base-content" />
                    </Link>
                    <Link
                        href="https://cogwheel.zone"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="tooltip"
                        data-tip="Visit our website"
                    >
                        <FaGlobe className="size-10 text-base-content" />
                    </Link>
                </div>
                <div>
                    {/*Spectra's Pathfinder RPC Logo*/}
                    <h4 className="text-base-content">Powered By:</h4>
                    <Link
                        href="https://docs.cogwheel.zone/spectra-ibc"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="hover:opacity-80 transition-opacity duration-300"
                    >
                        {/*Use spectra logo for now but another should take place */}
                        <Image
                            src="/pathfinder.png"
                            alt="Spectra's Pathfinder RPC Logo"
                            width={1200}
                            height={500}
                            className="w-40 xl:w-44 2xl:w-48"
                            loading="eager"
                        />
                    </Link>
                </div>
            </div>
            <div className="flex flex-row items-center justify-center space-x-4">
                <p className="text-sm text-center">
                    © {year} Cogwheel Validator. All rights reserved.
                </p>
                <Link
                    href="https://cogwheel.zone/terms-of-use-2"
                    target="_blank"
                    rel="noopener noreferrer"
                >
                    <p className="text-sm text-center underline">Terms of Use</p>
                </Link>
                <Link
                    href="https://cogwheel.zone/privacy-policy-3"
                    target="_blank"
                    rel="noopener noreferrer"
                >
                    <p className="text-sm text-center underline">Privacy Policy</p>
                </Link>
            </div>
        </footer>
    );
}
