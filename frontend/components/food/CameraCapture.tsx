'use client';
import { useEffect, useRef, useState } from 'react';
import TapTarget from '@/components/ui/TapTarget';

interface Props {
  onCapture: (file: File) => void;
  onClose: () => void;
}

// In-app camera capture. Always encodes via canvas.toBlob('image/jpeg') so
// the output format is guaranteed to be one the upload endpoint accepts,
// regardless of what the device's camera stream natively produces.
export default function CameraCapture({ onCapture, onClose }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // navigator.mediaDevices is undefined outside a secure context (HTTPS or
    // localhost) — calling .getUserMedia on it throws synchronously, before
    // any promise exists, so a .catch() below can't handle it. Guard first.
    if (!navigator.mediaDevices?.getUserMedia) {
      setError(
        'Camera capture needs a secure (HTTPS) connection. Use "Choose Photo" instead, or open this site over HTTPS.'
      );
      return;
    }

    let cancelled = false;
    navigator.mediaDevices
      .getUserMedia({ video: { facingMode: 'environment' } })
      .then(stream => {
        if (cancelled) {
          stream.getTracks().forEach(t => t.stop());
          return;
        }
        streamRef.current = stream;
        if (videoRef.current) videoRef.current.srcObject = stream;
      })
      .catch(() => setError('Could not access the camera. Check browser permissions.'));

    return () => {
      cancelled = true;
      streamRef.current?.getTracks().forEach(t => t.stop());
    };
  }, []);

  const capture = () => {
    const video = videoRef.current;
    const canvas = canvasRef.current;
    if (!video || !canvas) return;
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.drawImage(video, 0, 0);
    canvas.toBlob(
      blob => {
        if (!blob) return;
        onCapture(new File([blob], 'meal.jpg', { type: 'image/jpeg' }));
      },
      'image/jpeg',
      0.9
    );
  };

  return (
    // max-h-dvh clamps to the *dynamic* viewport, not the large one a fixed
    // inset-0 element resolves to by default — on a mobile browser showing
    // its URL bar, "large" includes space the user can't actually see, so a
    // card sized to it would overflow the visible screen. The bottom padding
    // uses a literal env() term rather than --edge-inset-b: ADR-008 leaves
    // full-screen overlays like this one outside the --nav-block rule
    // (covering the nav bar is the point), so nothing underneath absorbs the
    // inset for this overlay, and --edge-inset-b is 0px below the sm
    // breakpoint exactly because the nav bar normally does that job instead.
    <div
      data-testid="camera-capture-overlay"
      className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center max-h-dvh px-4 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))]"
    >
      <div
        data-testid="camera-capture-card"
        className="bg-white dark:bg-gray-800 rounded-xl shadow-lg max-w-md w-full max-h-full overflow-hidden flex flex-col"
      >
        <div className="shrink-0 px-4 py-3 border-b border-gray-100 dark:border-gray-700 flex justify-between items-center">
          <p className="text-sm font-semibold text-gray-900 dark:text-white">Take a photo</p>
          <TapTarget
            onClick={onClose}
            className="flex items-center justify-center text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-sm"
          >
            Cancel
          </TapTarget>
        </div>
        {/* flex-1 lets this region give up height when the viewport is short;
            min-h-0 is what actually allows that shrink — a flex item's
            automatic minimum size is its content size, so without min-h-0
            the video would refuse to shrink below its natural height and the
            column would overflow exactly as it did before this region
            existed. overflow-y-auto is the last-resort fallback for a
            viewport too short even for a fully shrunk preview; the Capture
            button lives in the footer below, outside this scroll region, so
            it can never be scrolled off screen. */}
        <div className="flex-1 min-h-0 overflow-y-auto p-4">
          {error ? (
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          ) : (
            // object-contain letterboxes a shrunk preview against the black
            // background instead of squashing it; capture() below still
            // draws from the video's own videoWidth/videoHeight, so the
            // captured image is unaffected by how small the preview renders.
            // eslint-disable-next-line jsx-a11y/media-has-caption
            <video
              ref={videoRef}
              autoPlay
              playsInline
              muted
              className="w-full h-full object-contain rounded-lg bg-black"
            />
          )}
          <canvas ref={canvasRef} className="hidden" />
        </div>
        {!error && (
          <div className="shrink-0 px-4 pb-4">
            <TapTarget
              onClick={capture}
              className="w-full rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white"
            >
              Capture
            </TapTarget>
          </div>
        )}
      </div>
    </div>
  );
}
