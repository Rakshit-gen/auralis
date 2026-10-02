"use client";

export default function Error({ reset }: { error: Error; reset: () => void }) {
  return (
    <div className="container-page flex min-h-[60vh] flex-col items-center justify-center text-center">
      <h1 className="font-display text-2xl text-bone-100">This page didn&rsquo;t load</h1>
      <p className="mt-2 max-w-sm text-sm text-bone-300">
        A request failed before the page finished loading. It is usually temporary, so a reload often fixes it.
      </p>
      <button onClick={reset} className="btn-primary mt-6">
        Reload this page
      </button>
    </div>
  );
}
