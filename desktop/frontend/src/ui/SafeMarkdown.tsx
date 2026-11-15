import {
  isValidElement,
  memo,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import Markdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";
import Icon from "../Icon";

// Plain text of a rendered code block, for copying.
function textOf(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (isValidElement<{ children?: ReactNode }>(node))
    return textOf(node.props.children);
  return "";
}

function languageOf(node: ReactNode): string {
  if (!isValidElement<{ className?: string }>(node)) return "";
  return /language-([\w+#.-]+)/.exec(node.props.className || "")?.[1] || "";
}

function CodeBlock({ children }: { children?: ReactNode }) {
  const [copied, setCopied] = useState<"" | "Copied" | "Copy failed">("");
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(""), 1600);
    return () => clearTimeout(timer);
  }, [copied]);
  const language = languageOf(Array.isArray(children) ? children[0] : children);
  return (
    <div className="code-block">
      <div className="code-block-bar">
        <span>{language || "text"}</span>
        <button
          aria-label="Copy code"
          onClick={() => {
            navigator.clipboard
              .writeText(textOf(children).replace(/\n$/, ""))
              .then(() => setCopied("Copied"))
              .catch(() => setCopied("Copy failed"));
          }}
        >
          <Icon name={copied === "Copied" ? "check" : "copy"} size={12} />
          <span role="status">{copied || "Copy"}</span>
        </button>
      </div>
      <pre>{children}</pre>
    </div>
  );
}

/**
 * Agent and transcript text. Rendered without active HTML, links or remote
 * images: links become plain text and images a placeholder. Tables, task
 * lists and strikethrough are supported, and fenced code is highlighted when
 * its language is named.
 */
export const SafeMarkdown = memo(function SafeMarkdown({
  text,
  imageNote = "[Image omitted]",
}: {
  text: string;
  imageNote?: string;
}) {
  return (
    <Markdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[[rehypeHighlight, { detect: false }]]}
      components={{
        a: ({ children }) => (
          <span className="transcript-link">{children}</span>
        ),
        img: () => <span className="image-note">{imageNote}</span>,
        pre: ({ children }) => <CodeBlock>{children}</CodeBlock>,
        // Task list boxes show state only; they are not controls.
        input: ({ checked }) => (
          <span
            className={`task-box ${checked ? "done" : ""}`}
            role="img"
            aria-label={checked ? "Done" : "Not done"}
          >
            {checked && <Icon name="check" size={10} />}
          </span>
        ),
        table: ({ children }) => (
          <div className="table-scroll">
            <table>{children}</table>
          </div>
        ),
      }}
    >
      {text}
    </Markdown>
  );
});
