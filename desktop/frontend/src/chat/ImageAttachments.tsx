import { useEffect, useRef, useState } from "react";
import Icon from "../Icon";
import type { ChatImage } from "../api";

export const MAX_IMAGES = 6;
export const MAX_IMAGE_BYTES = 5 * 1024 * 1024;
const TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

type Attached = ChatImage & { preview: string };

// FileReader is available everywhere the app runs, including older WebKit.
function toBase64(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    // A data URL is "data:<type>;base64,<bytes>"; keep the bytes.
    reader.onload = () => resolve(String(reader.result).split(",")[1] || "");
    reader.readAsDataURL(file);
  });
}

/**
 * Images attached to the message being written. They are kept in memory until
 * the message is sent; unlike the text, they are not saved as a draft.
 */
export function useImageAttachments() {
  const [images, setImages] = useState<Attached[]>([]);
  const [error, setError] = useState("");
  const current = useRef<Attached[]>([]);
  current.current = images;
  // Release the previews when the conversation view goes away.
  useEffect(
    () => () =>
      current.current.forEach((image) => URL.revokeObjectURL(image.preview)),
    [],
  );

  async function add(files: Iterable<File>) {
    const problems: string[] = [];
    const accepted: Attached[] = [];
    for (const file of files) {
      const name = file.name || "Pasted image";
      if (!TYPES.includes(file.type))
        problems.push(`${name} is not a PNG, JPEG, GIF or WebP image.`);
      else if (file.size > MAX_IMAGE_BYTES)
        problems.push(`${name} is larger than 5 MB.`);
      else if (current.current.length + accepted.length >= MAX_IMAGES)
        problems.push(`Only ${MAX_IMAGES} images fit in one message.`);
      else
        try {
          accepted.push({
            name,
            data: await toBase64(file),
            preview: URL.createObjectURL(file),
          });
        } catch {
          problems.push(`${name} could not be read.`);
        }
    }
    if (accepted.length) setImages((items) => [...items, ...accepted]);
    setError([...new Set(problems)].join(" "));
  }
  function remove(index: number) {
    setImages((items) => {
      URL.revokeObjectURL(items[index]?.preview);
      return items.filter((_, i) => i !== index);
    });
    setError("");
  }
  function clear() {
    current.current.forEach((image) => URL.revokeObjectURL(image.preview));
    setImages([]);
    setError("");
  }
  return {
    images,
    error,
    add,
    remove,
    clear,
    /** What to send: names and bytes only. */
    payload: (): ChatImage[] =>
      images.map(({ name, data }) => ({ name, data })),
  };
}

/** The attached images above the composer toolbar, each removable. */
export function AttachedImages({
  images,
  error,
  disabled,
  onRemove,
}: {
  images: Attached[];
  error: string;
  disabled: boolean;
  onRemove: (index: number) => void;
}) {
  if (!images.length && !error) return null;
  return (
    <div className="attached-images">
      {images.length > 0 && (
        <ul aria-label="Attached images">
          {images.map((image, index) => (
            <li key={image.preview}>
              <img src={image.preview} alt="" />
              <span title={image.name}>{image.name}</span>
              <button
                type="button"
                aria-label={`Remove ${image.name}`}
                disabled={disabled}
                onClick={() => onRemove(index)}
              >
                <Icon name="close" size={11} />
              </button>
            </li>
          ))}
        </ul>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}

/** Image files in a paste or drop, if any. */
export function imageFiles(data: DataTransfer | null) {
  return [...(data?.files || [])].filter((file) =>
    file.type.startsWith("image/"),
  );
}
