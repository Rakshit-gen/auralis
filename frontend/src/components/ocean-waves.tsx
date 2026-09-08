"use client";

import { useEffect, useRef } from "react";

/**
 * The moving backdrop for the whole app: a field of overlapping swells drawn on
 * a single canvas, fixed behind the content. It is meant to be felt more than
 * looked at, so it stays low-contrast and slow.
 *
 * Cost control: the canvas renders at a capped pixel ratio, the loop stops when
 * the tab is hidden, and the whole thing falls back to a still frame when the
 * viewer has asked their system for reduced motion.
 */

type Wave = {
  amplitude: number;
  wavelength: number;
  speed: number;
  y: number; // vertical anchor as a fraction of viewport height
  color: string;
  lineWidth: number;
};

const WAVES: Wave[] = [
  { amplitude: 26, wavelength: 0.9, speed: 0.012, y: 0.34, color: "rgba(84, 208, 204, 0.10)", lineWidth: 1.5 },
  { amplitude: 34, wavelength: 1.3, speed: -0.008, y: 0.52, color: "rgba(84, 208, 204, 0.08)", lineWidth: 1.2 },
  { amplitude: 46, wavelength: 1.8, speed: 0.006, y: 0.68, color: "rgba(112, 179, 201, 0.09)", lineWidth: 1 },
  { amplitude: 62, wavelength: 2.6, speed: -0.004, y: 0.84, color: "rgba(226, 162, 77, 0.07)", lineWidth: 1 },
  { amplitude: 90, wavelength: 3.4, speed: 0.003, y: 1.02, color: "rgba(226, 162, 77, 0.05)", lineWidth: 1 },
];

export function OceanWaves() {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
    let width = window.innerWidth;
    let height = window.innerHeight;
    let raf = 0;
    let t = 0;

    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
      width = window.innerWidth;
      height = window.innerHeight;
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };

    const drawWave = (w: Wave, phase: number) => {
      const base = height * w.y;
      const step = 14;
      ctx.beginPath();
      for (let x = -step; x <= width + step; x += step) {
        const k = (x / width) * Math.PI * 2 * w.wavelength;
        const y =
          base +
          Math.sin(k + phase) * w.amplitude +
          Math.sin(k * 0.5 - phase * 1.3) * (w.amplitude * 0.35);
        if (x === -step) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      }
      ctx.strokeStyle = w.color;
      ctx.lineWidth = w.lineWidth;
      ctx.stroke();

      ctx.lineTo(width + step, height + 120);
      ctx.lineTo(-step, height + 120);
      ctx.closePath();
      const grad = ctx.createLinearGradient(0, base - w.amplitude, 0, height);
      grad.addColorStop(0, w.color);
      grad.addColorStop(1, "rgba(4, 7, 12, 0)");
      ctx.fillStyle = grad;
      ctx.fill();
    };

    const renderStill = () => {
      ctx.clearRect(0, 0, width, height);
      WAVES.forEach((w) => drawWave(w, w.y * 6));
    };

    const frame = () => {
      ctx.clearRect(0, 0, width, height);
      t += 1;
      WAVES.forEach((w) => drawWave(w, t * w.speed));
      raf = requestAnimationFrame(frame);
    };

    const start = () => {
      cancelAnimationFrame(raf);
      if (reduced.matches) {
        renderStill();
        return;
      }
      raf = requestAnimationFrame(frame);
    };

    const onResize = () => {
      resize();
      if (reduced.matches || document.hidden) renderStill();
    };

    const onVisibility = () => {
      if (document.hidden) cancelAnimationFrame(raf);
      else start();
    };

    resize();
    start();
    window.addEventListener("resize", onResize);
    document.addEventListener("visibilitychange", onVisibility);
    reduced.addEventListener?.("change", start);

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", onResize);
      document.removeEventListener("visibilitychange", onVisibility);
      reduced.removeEventListener?.("change", start);
    };
  }, []);

  return (
    <canvas
      ref={ref}
      aria-hidden
      className="pointer-events-none fixed inset-0 -z-10 h-full w-full opacity-80"
    />
  );
}
