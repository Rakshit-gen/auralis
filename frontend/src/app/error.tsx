"use client";

export default function Error({ reset }: { error: Error; reset: () => void }) {
  return (
    <div className="container-page flex min-h-[60vh] flex-col items-center justify-center text-center">
      <h1 className="font-display text-2xl text-bone-100">Something went wrong on this page</h1>
      <p className="mt-2 max-w-sm text-sm text-bone-300">
        The request did not complete. This is usually temporary.
      </p>
      <button onClick={reset} className="btn-primary mt-6">
        Reload this page
      </button>
    </div>
  );
}
