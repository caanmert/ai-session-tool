import { useCallback, useEffect, useRef } from "react";
import { useStored } from "../ui/stored";

/** A message written while the agent works, sent when its turn ends. */
export type Queued = {
  text: string;
  /** Why it was not sent. A held message is sent only when you choose to. */
  held?: string;
};

const isQueued = (value: unknown): value is Queued | null =>
  value === null ||
  (typeof value === "object" &&
    typeof (value as Queued).text === "string" &&
    ["undefined", "string"].includes(typeof (value as Queued).held));

const working = (status: string) =>
  ["running", "waiting", "stopping"].includes(status);

export const HELD_RESTART =
  "Not sent: ais closed before the response ended. Send it when you are ready.";
export const HELD_STOPPED = "Not sent: the response was stopped.";
export const HELD_ERROR = "Not sent: the response ended with an error.";
export const HELD_FAILED = "Not sent: it could not be sent.";

/**
 * One queued message per conversation, kept on this Mac so it is never lost.
 * It is sent only when this window sees the agent's turn end normally. A turn
 * that stops or fails, or a message queued before ais restarted, holds it for
 * you to send, edit or discard: opening saved work never starts a turn.
 */
export function useQueuedMessage(
  chatId: string,
  status: string,
  send: (text: string) => Promise<boolean>,
) {
  const [queued, setQueued] = useStored<Queued | null>(
    `ais:chat-queued:v1:${chatId}`,
    null,
    isQueued,
  );
  // A message left from an earlier run of the app is held, not sent.
  useEffect(() => {
    setQueued((item) =>
      item && !item.held ? { ...item, held: HELD_RESTART } : item,
    );
    // Only when the conversation is first shown.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chatId]);
  const latest = useRef(queued);
  latest.current = queued;
  const sendRef = useRef(send);
  sendRef.current = send;
  const previous = useRef(status);
  const sending = useRef(false);
  useEffect(() => {
    const was = previous.current;
    previous.current = status;
    const item = latest.current;
    if (!item || item.held || !working(was) || working(status)) return;
    if (status !== "idle") {
      setQueued({
        ...item,
        held: status === "stopped" ? HELD_STOPPED : HELD_ERROR,
      });
      return;
    }
    if (sending.current) return;
    sending.current = true;
    void sendRef.current(item.text).then((sent) => {
      sending.current = false;
      setQueued(sent ? null : { ...item, held: HELD_FAILED });
    });
  }, [status, setQueued]);

  /** Queue text after any message already queued; the whole message is then
   * sent when the turn ends, even if part of it was held. */
  const add = useCallback(
    (text: string) =>
      setQueued((item) => ({
        text: item ? `${item.text}\n\n${text}` : text,
      })),
    [setQueued],
  );
  const clear = useCallback(() => setQueued(null), [setQueued]);
  return { queued, add, clear };
}
