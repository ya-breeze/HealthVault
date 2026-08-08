'use client';
import { useEffect, useRef, useState } from 'react';

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
    <div className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-lg max-w-md w-full overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-100 dark:border-gray-700 flex justify-between items-center">
          <p className="text-sm font-semibold text-gray-900 dark:text-white">Take a photo</p>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-sm"
          >
            Cancel
          </button>
        </div>
        <div className="p-4">
          {error ? (
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          ) : (
            // eslint-disable-next-line jsx-a11y/media-has-caption
            <video ref={videoRef} autoPlay playsInline muted className="w-full rounded-lg bg-black" />
          )}
          <canvas ref={canvasRef} className="hidden" />
          {!error && (
            <button
              onClick={capture}
              className="mt-4 w-full py-2.5 rounded-lg text-sm font-medium bg-blue-600 hover:bg-blue-700 text-white"
            >
              Capture
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
