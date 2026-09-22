import { useState } from "react";

import { authenticatedMediaUrl } from "@/lib/media-urls";

type AuthenticatedImageProps = {
  src: string;
  alt: string;
  className?: string;
  onError?: () => void;
};

/** Authenticated media image; hides on failure and notifies parent for letter fallbacks. */
export function AuthenticatedImage({ src, alt, className, onError }: AuthenticatedImageProps) {
  const [failed, setFailed] = useState(false);
  if (failed) {
    return null;
  }

  return (
    <img
      src={authenticatedMediaUrl(src)}
      alt={alt}
      className={className}
      loading="lazy"
      decoding="async"
      onError={() => {
        setFailed(true);
        onError?.();
      }}
    />
  );
}
