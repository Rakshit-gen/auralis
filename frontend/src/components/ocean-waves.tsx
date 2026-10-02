"use client";

import { useEffect, useRef } from "react";

/** A slow field of sound ribbons. Decorative, time-based, capped at 30fps. */
export function OceanWaves() {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    const ctx = canvas?.getContext("2d");
    if (!canvas || !ctx) return;
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    let width = 0, height = 0, raf = 0, last = 0, elapsed = 0;
    const pointer = { x: 0, y: 0 };
    const eased = { x: 0, y: 0 };

    const paint = () => {
      ctx.clearRect(0, 0, width, height);
      const t = elapsed / 1000;
      const mobile = width < 768;
      // Each ribbon is a family of closely spaced strands, like a voice's harmonics.
      for (let band = 0; band < 3; band++) {
        const gradient = ctx.createLinearGradient(0, 0, width, height);
        gradient.addColorStop(0, "rgba(77, 181, 190, 0)");
        gradient.addColorStop(.28, "rgba(73, 180, 183, .12)");
        gradient.addColorStop(.62, band === 1 ? "rgba(219, 167, 100, .24)" : "rgba(121, 224, 212, .23)");
        gradient.addColorStop(1, "rgba(86, 132, 187, .02)");
        ctx.strokeStyle = gradient;
        ctx.lineWidth = .65;
        const strands = mobile ? 16 : 28;
        for (let line = 0; line < strands; line++) {
          ctx.beginPath();
          for (let step = 0; step <= 110; step++) {
            const u = step / 110;
            const x = u * width;
            const envelope = Math.sin(u * Math.PI);
            const y = height * (.48 + band * .23)
              + Math.sin(u * 6.2 + t * .12 + band * 1.6) * height * .14
              + Math.cos(u * 10 - t * .08 + band) * height * .045 * envelope
              + (line - strands / 2) * (3 + 8 * envelope * envelope)
              + eased.y * envelope * 14 + eased.x * Math.sin(u * 4) * 12;
            if (step === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
          }
          ctx.stroke();
        }
      }
      // Sparse dust, never a grid behind the text.
      for (let i = 0; i < (mobile ? 25 : 65); i++) {
        const x = ((i * 137.508 + 31) % 997) / 997 * width;
        const y = ((i * 73.31 + 17) % 991) / 991 * height;
        const opacity = .08 + (Math.sin(t * .4 + i * 2) + 1) * .09;
        ctx.fillStyle = `rgba(185, 217, 210, ${opacity})`;
        ctx.beginPath(); ctx.arc(x, y, i % 7 === 0 ? 1.2 : .65, 0, Math.PI * 2); ctx.fill();
      }
    };
    const resize = () => {
      width = innerWidth; height = innerHeight;
      const dpr = Math.min(devicePixelRatio || 1, 1.5);
      canvas.width = Math.round(width * dpr); canvas.height = Math.round(height * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      paint();
    };
    const frame = (now: number) => {
      if (now - last >= 1000 / 30) {
        elapsed += Math.min(now - last, 60); last = now;
        eased.x += (pointer.x - eased.x) * .025;
        eased.y += (pointer.y - eased.y) * .025;
        paint();
      }
      raf = requestAnimationFrame(frame);
    };
    const start = () => {
      cancelAnimationFrame(raf);
      if (reduced.matches || document.hidden) { paint(); return; }
      last = performance.now(); raf = requestAnimationFrame(frame);
    };
    const move = (event: PointerEvent) => {
      if (reduced.matches || event.pointerType !== "mouse") return;
      pointer.x = event.clientX / width - .5;
      pointer.y = event.clientY / height - .5;
    };
    resize(); start();
    addEventListener("resize", resize);
    addEventListener("pointermove", move, { passive: true });
    document.addEventListener("visibilitychange", start);
    reduced.addEventListener("change", start);
    return () => {
      cancelAnimationFrame(raf);
      removeEventListener("resize", resize);
      removeEventListener("pointermove", move);
      document.removeEventListener("visibilitychange", start);
      reduced.removeEventListener("change", start);
    };
  }, []);
  return <canvas ref={ref} aria-hidden="true" className="soundscape-canvas" />;
}
