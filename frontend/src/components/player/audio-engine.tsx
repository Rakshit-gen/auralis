"use client";

import { useEffect, useRef } from "react";
import Hls from "hls.js";
import { usePlayer } from "@/stores/player";

/**
 * The single <audio> element for the whole app. It loads HLS via hls.js where
 * MSE is available and falls back to native HLS (Safari). All transport state
 * is driven by the player store; this component only wires the element to it.
 */
export function AudioEngine() {
  const ref = useRef<HTMLAudioElement | null>(null);
  const hlsRef = useRef<Hls | null>(null);

  const authorization = usePlayer((s) => s.authorization);
  const playing = usePlayer((s) => s.playing);
  const rate = usePlayer((s) => s.rate);
  const volume = usePlayer((s) => s.volume);
  const seekRequest = usePlayer((s) => s.seekRequest);

  const onTimeUpdate = usePlayer((s) => s.onTimeUpdate);
  const onBuffering = usePlayer((s) => s.onBuffering);
  const onEnded = usePlayer((s) => s.onEnded);
  const clearSeek = usePlayer((s) => s.clearSeek);
  const setPlaying = usePlayer((s) => s.setPlaying);

  // Load a new source when the authorization (and therefore the signed URL) changes.
  useEffect(() => {
    const el = ref.current;
    if (!el || !authorization) return;
    const src = authorization.hls_master_url;

    hlsRef.current?.destroy();
    hlsRef.current = null;

    if (Hls.isSupported()) {
      const hls = new Hls({ enableWorker: true, lowLatencyMode: false, backBufferLength: 60 });
      hlsRef.current = hls;
      hls.loadSource(src);
      hls.attachMedia(el);
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        el.currentTime = authorization.resume_position_sec || 0;
        void el.play().catch(() => setPlaying(false));
      });
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (data.fatal) {
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR) hls.startLoad();
          else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) hls.recoverMediaError();
          else setPlaying(false);
        }
      });
    } else {
      el.src = src;
      el.addEventListener(
        "loadedmetadata",
        () => {
          el.currentTime = authorization.resume_position_sec || 0;
          void el.play().catch(() => setPlaying(false));
        },
        { once: true },
      );
    }

    return () => {
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
  }, [authorization, setPlaying]);

  // Reflect store transport state onto the element.
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (playing) void el.play().catch(() => setPlaying(false));
    else el.pause();
  }, [playing, setPlaying]);

  useEffect(() => {
    if (ref.current) ref.current.playbackRate = rate;
  }, [rate]);

  useEffect(() => {
    if (ref.current) ref.current.volume = volume;
  }, [volume]);

  useEffect(() => {
    const el = ref.current;
    if (el && seekRequest != null) {
      el.currentTime = seekRequest;
      clearSeek();
    }
  }, [seekRequest, clearSeek]);

  const previewLimit = authorization?.preview_only ? authorization.preview_limit_sec : 0;

  return (
    <audio
      ref={ref}
      preload="metadata"
      onTimeUpdate={(e) => {
        const el = e.currentTarget;
        if (previewLimit && el.currentTime >= previewLimit) {
          el.pause();
          setPlaying(false);
          el.currentTime = Math.max(0, previewLimit - 1);
          return;
        }
        onTimeUpdate(el.currentTime, el.duration);
      }}
      onWaiting={() => onBuffering(true)}
      onPlaying={() => onBuffering(false)}
      onCanPlay={() => onBuffering(false)}
      onEnded={onEnded}
      className="hidden"
    />
  );
}
