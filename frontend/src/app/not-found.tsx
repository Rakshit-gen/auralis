import Link from "next/link";

export default function NotFound() {
  return (
    <div className="container-page flex min-h-[60vh] flex-col items-center justify-center text-center">
      <p className="font-display text-6xl text-ink-600">404</p>
      <h1 className="mt-4 font-display text-2xl text-bone-100">We could not find that page</h1>
      <p className="mt-2 text-sm text-bone-300">The story you are looking for may have been unpublished or moved.</p>
      <Link href="/" className="btn-primary mt-6">
        Back to Auralis
      </Link>
    </div>
  );
}
