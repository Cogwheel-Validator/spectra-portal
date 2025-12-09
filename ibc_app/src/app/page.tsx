import Image from "next/image";
import Link from "next/link";
import { FlyInButton } from "@/components/motion/flyInButton";
import { Particles } from "@/components/ui/particles";

export default function Home() {
    return (
        <div className="relative min-h-[80dvh] bg-blend-soft-light bg-radial-[at_50%_65%] from-slate-800 via-blue-950 to-indigo-950 to-90%">
            <Particles className="absolute inset-0 z-0" />
            <div className="relative z-10 flex max-w-5xl mx-auto flex-col items-center justify-center min-h-screen">
                <div className="flex flex-col items-center justify-center space-y-4 text-base-content">
                    <Image
                        src="/spectra_logo.png"
                        alt="Spectra Logo"
                        className="size-28"
                        loading="eager"
                        width={360}
                        height={360}
                    />
                    <h1 className="text-5xl text-center">The Spectra IBC Hub</h1>
                    <p className="max-w-4xl font-semibold leading-relaxed text-center">
                        Send your assets accross different chains using Inter Blockchain
                        Communication. With easy auto routing available via the{" "}
                        <u className="font-extrabold">Spectra Solver RPC</u>, sending assets has
                        never been easier.
                        <br />
                        <br />
                        Send your assets to another chain using the Spectra 💫 IBC!
                    </p>

                    <FlyInButton type="button" className="btn btn-primary btn-lg lg:btn-xl">
                        <Link href="/ibc">IBC</Link>
                    </FlyInButton>
                </div>
            </div>
        </div>
    );
}
