"use client";

import { useEffect, useRef } from "react";

/**
 * The moving backdrop for the whole app. Two layers on one canvas, fixed behind
 * the content:
 *
 *   1. A field of bioluminescent points that brighten and fade in a slow
 *      diagonal swell, so the dark water always has some life in it.
 *   2. A stack of overlapping wave crests low on the screen, felt more than
 *      looked at.
 *
 * Cost control: the canvas renders at a capped pixel ratio, the point grid is
 * spaced so even a large screen stays a few thousand cells, the loop stops when
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
  { amplitude: 24, wavelength: 0.9, speed: 0.013, y: 0.42, color: "rgba(120, 226, 222, 0.16)", lineWidth: 1.4 },
  { amplitude: 32, wavelength: 1.3, speed: -0.009, y: 0.57, color: "rgba(84, 208, 204, 0.15)", lineWidth: 1.2 },
  { amplitude: 44, wavelength: 1.8, speed: 0.006, y: 0.71, color: "rgba(112, 179, 201, 0.16)", lineWidth: 1.1 },
  { amplitude: 60, wavelength: 2.6, speed: -0.0045, y: 0.85, color: "rgba(226, 162, 77, 0.13)", lineWidth: 1.1 },
  { amplitude: 88, wavelength: 3.4, speed: 0.003, y: 1.02, color: "rgba(226, 162, 77, 0.1)", lineWidth: 1 },
];

const GRID = 22; // px between points
const DOT = 2; // px drawn per point

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
    let cols = 0;
    let rows = 0;
    let raf = 0;
    let t = 0;

    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 1.5);
      width = window.innerWidth;
      height = window.innerHeight;
      cols = Math.ceil(width / GRID) + 1;
      rows = Math.ceil(height / GRID) + 1;
      canvas.width = Math.floor(width * dpr);
      canvas.height = Math.floor(height * dpr);
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };

    // A diagonal wave of brightness rolling across a fixed grid, with a faster
    // twinkle on top so individual points visibly pulse. Points near a crest
    // glow; the rest sit low but never fully dark.
    const drawField = (phase: number) => {
      for (let cy = 0; cy < rows; cy++) {
        for (let cx = 0; cx < cols; cx++) {
          const x = cx * GRID;
          const y = cy * GRID;
          const swell =
            Math.sin(x * 0.012 + y * 0.016 + phase) +
            Math.sin(x * 0.026 - y * 0.008 - phase * 0.6) * 0.5;
          // per-point shimmer: each cell breathes on its own offset
          const twinkle = Math.sin(phase * 3.2 + cx * 1.7 + cy * 0.9) * 0.45;
          const lit = (swell + twinkle + 1.5) / 3; // 0..1-ish
          if (lit < 0.22) continue;
          const a = Math.min(0.72, (lit - 0.22) * 1.35);
          // warm points ride the far side of the swell, cool ones the near side
          ctx.fillStyle =
            swell > 0.75 ? `rgba(240, 197, 128, ${a})` : `rgba(126, 231, 226, ${a})`;
          const s = lit > 0.8 ? DOT + 1.5 : lit > 0.6 ? DOT + 0.5 : DOT;
          ctx.fillRect(x, y, s, s);
        }
      }
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

    const paint = (fieldPhase: number, wavePhase: (w: Wave) => number) => {
      ctx.clearRect(0, 0, width, height);
      drawField(fieldPhase);
      WAVES.forEach((w) => drawWave(w, wavePhase(w)));
    };

    const renderStill = () => paint(1.4, (w) => w.y * 6);

    const frame = () => {
      t += 1;
      paint(t * 0.011, (w) => t * w.speed);
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
      className="pointer-events-none fixed inset-0 -z-10 h-full w-full opacity-90"
    />
  );
}
