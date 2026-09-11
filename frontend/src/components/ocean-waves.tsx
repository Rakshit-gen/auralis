"use client";

import { useEffect, useRef } from "react";

/**
 * The moving backdrop for the whole app. Two layers on one canvas, fixed behind
 * the content:
 *
 *   1. A field of bioluminescent points that brighten and fade in a slow
 *      diagonal swell, so the dark water always has some life in it. Moving
 *      the pointer near them makes them flare, the way real bioluminescent
 *      plankton lights up when disturbed.
 *   2. A stack of overlapping wave crests low on the screen, felt more than
 *      looked at.
 *
 * Cost control: the canvas renders at a capped pixel ratio, the point grid is
 * spaced so even a large screen stays a few thousand cells, the loop stops when
 * the tab is hidden, and the whole thing falls back to a still frame when the
 * viewer has asked their system for reduced motion (which also turns off the
 * pointer effect).
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

type Ripple = { x: number; y: number; born: number; strength: number };
const MAX_RIPPLES = 6;

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

    // Ring buffer of recent pointer touches; a point brightens as it ages in,
    // the way a disturbance ripples out and fades rather than switching on.
    const ripples: Ripple[] = Array.from({ length: MAX_RIPPLES }, () => ({ x: 0, y: 0, born: 0, strength: 0 }));
    let rippleIdx = 0;
    let lastRippleAt = 0;

    const addRipple = (x: number, y: number, strength: number) => {
      ripples[rippleIdx] = { x, y, born: performance.now(), strength };
      rippleIdx = (rippleIdx + 1) % MAX_RIPPLES;
    };

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
    const drawField = (phase: number, now: number) => {
      for (let cy = 0; cy < rows; cy++) {
        for (let cx = 0; cx < cols; cx++) {
          const x = cx * GRID;
          const y = cy * GRID;
          const swell =
            Math.sin(x * 0.012 + y * 0.016 + phase) +
            Math.sin(x * 0.026 - y * 0.008 - phase * 0.6) * 0.5;
          // per-point shimmer: each cell breathes on its own offset
          const twinkle = Math.sin(phase * 3.2 + cx * 1.7 + cy * 0.9) * 0.45;

          // A faint expanding, fading ring per recent pointer touch. Cheap:
          // most slots are unused (strength 0) and skip immediately.
          let glow = 0;
          for (const r of ripples) {
            if (r.strength <= 0) continue;
            const age = (now - r.born) / 1000;
            if (age < 0 || age > 1.4) continue;
            const d = Math.hypot(x - r.x, y - r.y);
            const ring = Math.sin(d * 0.05 - age * 9) * Math.exp(-d * 0.012) * Math.exp(-age * 2.6);
            if (ring > 0) glow += ring * r.strength;
          }

          const lit = (swell + twinkle + 1.5) / 3 + glow * 0.3; // 0..1-ish
          if (lit < 0.22) continue;
          const a = Math.min(0.78, (lit - 0.22) * 1.35);
          // warm points ride the far side of the swell, cool ones the near
          // side; a touched point reads cool/teal regardless of swell phase
          // (real bioluminescence flares blue-green when disturbed).
          ctx.fillStyle =
            swell > 0.75 && glow < 0.1 ? `rgba(240, 197, 128, ${a})` : `rgba(126, 231, 226, ${a})`;
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
      drawField(fieldPhase, performance.now());
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

    // Window-level, not canvas: the canvas is pointer-events-none so the real
    // UI stays clickable, but window listeners still see every move over it.
    const onPointerMove = (e: PointerEvent) => {
      if (reduced.matches) return;
      const now = performance.now();
      if (now - lastRippleAt < 110) return;
      lastRippleAt = now;
      addRipple(e.clientX, e.clientY, 0.55);
    };
    const onPointerDown = (e: PointerEvent) => {
      if (reduced.matches) return;
      addRipple(e.clientX, e.clientY, 1.1); // a firmer touch: bigger flare
    };

    resize();
    start();
    window.addEventListener("resize", onResize);
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pointermove", onPointerMove, { passive: true });
    window.addEventListener("pointerdown", onPointerDown, { passive: true });
    reduced.addEventListener?.("change", start);

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", onResize);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerdown", onPointerDown);
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
