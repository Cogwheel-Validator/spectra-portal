import type { Metadata } from "next";
import { Expletus_Sans, Roboto } from "next/font/google";
import "./globals.css";
import FooterInterface from "@/components/ui/footer/footerInterface";
import MenuInterface from "@/components/ui/menu/menuInterface";
import { TanstackProvider } from "@/context/tanstackProvider";
import { WalletProvider } from "@/context/walletContext";

const expletusSans = Expletus_Sans({
    variable: "--font-expletus-sans",
    subsets: ["latin"],
});

const robotoFont = Roboto({
    variable: "--font-roboto",
    subsets: ["latin"],
});

const url = new URL("https://portal.thespectra.io")
const title = "Spectra Portal - Transfer Asset Across Blockchains"
const description = "Spectra Portal is a web application that provides an interface for users to send and receive assets across different chains using the Inter Blockchain Communication Protocol."

export const metadata: Metadata = {
    title: title,
    description: description,
    keywords: [
        "IBC",
        "Inter Blockchain Communication",
        "Spectra",
        "Spectra Portal",
        "Spectra Explorer",
        "Cogwheel Validator",
        "Spectra Pathfinder",
    ],
    authors: [{ name: "Cogwheel Validator", url: "https://cogwheel.zone" }],
    metadataBase: url,
    openGraph: {
        url: url,
        title: title,
        description: description,
        images: [{ url: "/spectraPortalSEO.png"}]
    },
    twitter: {
        card: "summary_large_image",
        title: title,
        images: [{ url: "/spectraPortalSEO.png"}]
    },
    robots: {
        index: true,
        follow: true,
        googleBot: {
            index: true,
            follow: true,
        },
    },
};

export default function RootLayout({
    children,
}: Readonly<{
    children: React.ReactNode;
}>) {
    return (
        <html lang="en" className="h-full">
            <body
                className={`${expletusSans.variable} ${robotoFont.variable} antialiased min-h-full flex flex-col`}
            >
                <TanstackProvider>
                    <WalletProvider>
                        <MenuInterface />
                        <main className="flex-1 relative">
                            {children}
                            <FooterInterface />
                        </main>
                    </WalletProvider>
                </TanstackProvider>
            </body>
        </html>
    );
}
