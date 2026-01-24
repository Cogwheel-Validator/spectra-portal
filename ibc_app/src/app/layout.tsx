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

export const metadata: Metadata = {
    title: "Spectra Portal",
    description: "Spectra Portal",
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
