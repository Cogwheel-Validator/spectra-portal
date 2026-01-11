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
    title: "Spectra IBC Hub",
    description: "Spectra IBC Hub",
};

export default function RootLayout({
    children,
}: Readonly<{
    children: React.ReactNode;
}>) {
    return (
        <html lang="en">
            <body className={`${expletusSans.variable} ${robotoFont.variable} antialiased`}>
                <TanstackProvider>
                    <WalletProvider>
                        <MenuInterface />
                        {children}
                        <FooterInterface />
                    </WalletProvider>
                </TanstackProvider>
            </body>
        </html>
    );
}
