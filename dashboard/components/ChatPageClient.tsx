'use client';

// Full-page, ChatGPT-style chat over the trace data. Independent of the
// dashboard — the backend answers by running real store queries through the
// chat engine's tools, so replies are grounded in captured data.

import { useCallback, useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import IconButton from '@mui/material/IconButton';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import SendIcon from '@mui/icons-material/Send';
import StopIcon from '@mui/icons-material/Stop';
import StopCircleOutlinedIcon from '@mui/icons-material/StopCircleOutlined';
import ChatBubbleOutlineOutlinedIcon from '@mui/icons-material/ChatBubbleOutlineOutlined';
import AppHeader from './AppHeader';
import { sendChat, type ChatMessage } from '@/lib/api';
import { GOOGLE_BLUE, TEXT_SECONDARY, BORDER_GRAY } from '@/lib/theme';

// A thread entry: a chat message plus UI-only metadata. `stopped` marks a
// question the user abandoned — it stays visible with a mark but is excluded
// from the context sent to the assistant.
type ChatEntry = ChatMessage & { stopped?: boolean };

const EXAMPLES = [
  'Which agent has the most warnings, and what did it do?',
  'Show me the errors in the last hour.',
  'What did the misbehaving agent’s prompt say?',
  'Find traces that mention an external upload.',
];

export default function ChatPageClient() {
  const [messages, setMessages] = useState<ChatEntry[]>([]);
  const [input, setInput] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notConfigured, setNotConfigured] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);

  // Each send gets a monotonic id; activeReq holds the id whose result the UI
  // still wants. Stop bumps activeReq so a resolved-but-abandoned request is
  // dropped — the backend call keeps running server-side, the UI just ignores
  // it.
  const reqCounter = useRef(0);
  const activeReq = useRef<number | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, pending]);

  const send = useCallback(
    async (text: string) => {
      const q = text.trim();
      if (!q || pending) return;
      setError(null);
      // The context sent to the assistant excludes any stopped exchanges — a
      // question the user abandoned never becomes part of the conversation the
      // model sees.
      const context: ChatMessage[] = messages
        .filter((m) => !m.stopped)
        .map(({ role, content }) => ({ role, content }));
      context.push({ role: 'user', content: q });

      setMessages((m) => [...m, { role: 'user', content: q }]);
      setInput('');
      const id = ++reqCounter.current;
      activeReq.current = id;
      setPending(true);
      try {
        const reply = await sendChat(context);
        if (activeReq.current !== id) return; // stopped — drop the result
        setMessages((m) => [...m, { role: 'assistant', content: reply }]);
      } catch (e) {
        if (activeReq.current !== id) return; // stopped — ignore the error too
        const err = e as Error & { status?: number };
        if (err.status === 503) setNotConfigured(true);
        setError(err.message || 'request failed');
        // Roll the unanswered question back out so the user can retry/edit.
        setMessages((m) => m.slice(0, -1));
        setInput(q);
      } finally {
        if (activeReq.current === id) setPending(false);
      }
    },
    [messages, pending],
  );

  // Stop the current query. The question stays in the thread with a clear
  // "stopped" mark (not deleted), and — being marked — it is excluded from the
  // context of every later question. The in-flight API call is not aborted;
  // its result is simply discarded when it arrives.
  const stop = useCallback(() => {
    activeReq.current = null;
    setPending(false);
    setError(null);
    setMessages((m) => {
      if (!m.length || m[m.length - 1].role !== 'user') return m;
      const copy = m.slice();
      copy[copy.length - 1] = { ...copy[copy.length - 1], stopped: true };
      return copy;
    });
  }, []);

  const empty = messages.length === 0;

  const composer = (
    <Composer
      input={input}
      setInput={setInput}
      onSend={() => send(input)}
      onStop={stop}
      pending={pending}
    />
  );

  const configNotice = notConfigured && (
    <Alert severity="info" sx={{ mb: 2, textAlign: 'left' }}>
      Chat is not configured on the server. Set <code>OPENAI_API_KEY</code> on
      the api service (a <code>.env</code> file with
      <code> OPENAI_API_KEY=sk-…</code>, then <code>make up</code>) to enable it.
    </Alert>
  );

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <AppHeader />

      {empty ? (
        // Landing state: greeting + the input box centered in the middle, like
        // a home page. The same input becomes the bottom composer once the
        // conversation starts.
        <Box
          sx={{
            flex: 1,
            overflowY: 'auto',
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center',
            alignItems: 'center',
            px: 2,
          }}
        >
          <Box sx={{ width: '100%', maxWidth: 640, textAlign: 'center' }}>
            {configNotice}
            <ChatBubbleOutlineOutlinedIcon
              sx={{ fontSize: 40, color: GOOGLE_BLUE, mb: 1 }}
            />
            <Typography sx={{ fontSize: 24, fontWeight: 500, mb: 0.5 }}>
              Ask about your agent traces
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
              Answers are grounded in captured trace data — the assistant runs
              real queries, it doesn’t guess.
            </Typography>

            {composer}

            <Box
              sx={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: 1,
                justifyContent: 'center',
                mt: 2.5,
              }}
            >
              {EXAMPLES.map((ex) => (
                <Chip
                  key={ex}
                  label={ex}
                  variant="outlined"
                  onClick={() => send(ex)}
                  sx={{
                    cursor: 'pointer',
                    fontWeight: 400,
                    borderColor: BORDER_GRAY,
                    '&:hover': { bgcolor: '#f1f3f4' },
                  }}
                />
              ))}
            </Box>
            {error && !notConfigured && (
              <Alert severity="error" sx={{ mt: 2, textAlign: 'left' }}>
                {error}
              </Alert>
            )}
          </Box>
        </Box>
      ) : (
        <>
          <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 760, mx: 'auto', px: 2, py: 3 }}>
              {configNotice}
              {messages.map((m, i) => (
                <Bubble key={i} message={m} />
              ))}
              {pending && (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, my: 2 }}>
                  <CircularProgress size={16} />
                  <Typography variant="body2" color="text.secondary">
                    thinking…
                  </Typography>
                </Box>
              )}
              {error && !notConfigured && (
                <Alert severity="error" sx={{ mt: 1 }}>
                  {error}
                </Alert>
              )}
              <div ref={endRef} />
            </Box>
          </Box>

          {/* Composer pinned at the bottom once the conversation has started. */}
          <Box sx={{ borderTop: `1px solid ${BORDER_GRAY}`, bgcolor: '#fff' }}>
            <Box sx={{ maxWidth: 760, mx: 'auto', px: 2, py: 1.5 }}>{composer}</Box>
          </Box>
        </>
      )}
    </Box>
  );
}

// Composer is the input row (text field + send/stop) plus the key hint. It is
// rendered centered on the landing state and pinned at the bottom afterward.
function Composer({
  input,
  setInput,
  onSend,
  onStop,
  pending,
}: {
  input: string;
  setInput: (v: string) => void;
  onSend: () => void;
  onStop: () => void;
  pending: boolean;
}) {
  return (
    <Box sx={{ width: '100%' }}>
      <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-end' }}>
        <TextField
          fullWidth
          multiline
          maxRows={6}
          size="small"
          placeholder="Ask about agents, traces, errors, warnings…"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              onSend();
            }
          }}
          disabled={pending}
        />
        {pending ? (
          <IconButton
            onClick={onStop}
            aria-label="stop"
            sx={{ mb: 0.25, bgcolor: '#f1f3f4', '&:hover': { bgcolor: '#e8eaed' } }}
          >
            <StopIcon />
          </IconButton>
        ) : (
          <IconButton
            color="primary"
            onClick={onSend}
            disabled={input.trim() === ''}
            aria-label="send"
            sx={{ mb: 0.25 }}
          >
            <SendIcon />
          </IconButton>
        )}
      </Box>
      <Typography
        variant="caption"
        sx={{ display: 'block', textAlign: 'center', color: TEXT_SECONDARY, pt: 0.75 }}
      >
        Enter to send · Shift+Enter for a new line
      </Typography>
    </Box>
  );
}

function Bubble({ message }: { message: ChatEntry }) {
  const isUser = message.role === 'user';
  const stopped = !!message.stopped;
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: isUser ? 'flex-end' : 'flex-start',
        mb: 1.5,
      }}
    >
      <Paper
        sx={{
          px: 1.75,
          py: 1.25,
          maxWidth: '85%',
          bgcolor: stopped ? '#f1f3f4' : isUser ? '#e8f0fe' : '#ffffff',
          border: `1px solid ${stopped ? BORDER_GRAY : isUser ? '#d2e3fc' : BORDER_GRAY}`,
          opacity: stopped ? 0.75 : 1,
        }}
      >
        <Typography
          component="div"
          sx={{
            fontSize: 14,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            color: stopped ? TEXT_SECONDARY : '#202124',
          }}
        >
          {message.content}
        </Typography>
      </Paper>
      {stopped && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5 }}>
          <StopCircleOutlinedIcon sx={{ fontSize: 15, color: TEXT_SECONDARY }} />
          <Typography variant="caption" sx={{ color: TEXT_SECONDARY }}>
            Stopped — not sent to the assistant
          </Typography>
        </Box>
      )}
    </Box>
  );
}
