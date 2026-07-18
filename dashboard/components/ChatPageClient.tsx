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
import ChatBubbleOutlineOutlinedIcon from '@mui/icons-material/ChatBubbleOutlineOutlined';
import AppHeader from './AppHeader';
import { sendChat, type ChatMessage } from '@/lib/api';
import { GOOGLE_BLUE, TEXT_SECONDARY, BORDER_GRAY } from '@/lib/theme';

const EXAMPLES = [
  'Which agent has the most warnings, and what did it do?',
  'Show me the errors in the last hour.',
  'What did the misbehaving agent’s prompt say?',
  'Find traces that mention an external upload.',
];

export default function ChatPageClient() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notConfigured, setNotConfigured] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, pending]);

  const send = useCallback(
    async (text: string) => {
      const q = text.trim();
      if (!q || pending) return;
      setError(null);
      const next = [...messages, { role: 'user' as const, content: q }];
      setMessages(next);
      setInput('');
      setPending(true);
      try {
        const reply = await sendChat(next);
        setMessages((m) => [...m, { role: 'assistant', content: reply }]);
      } catch (e) {
        const err = e as Error & { status?: number };
        if (err.status === 503) setNotConfigured(true);
        setError(err.message || 'request failed');
        // Roll the unanswered question back out so the user can retry/edit.
        setMessages((m) => m.slice(0, -1));
        setInput(q);
      } finally {
        setPending(false);
      }
    },
    [messages, pending],
  );

  const empty = messages.length === 0;

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <AppHeader />

      <Box sx={{ flex: 1, overflowY: 'auto' }}>
        <Box sx={{ maxWidth: 760, mx: 'auto', px: 2, py: 3 }}>
          {notConfigured && (
            <Alert severity="info" sx={{ mb: 2 }}>
              Chat is not configured on the server. Set <code>OPENAI_API_KEY</code>{' '}
              on the api service (a <code>.env</code> file with
              <code> OPENAI_API_KEY=sk-…</code>, then <code>make up</code>) to
              enable it.
            </Alert>
          )}

          {empty ? (
            <Box sx={{ textAlign: 'center', mt: 8 }}>
              <ChatBubbleOutlineOutlinedIcon
                sx={{ fontSize: 40, color: GOOGLE_BLUE, mb: 1 }}
              />
              <Typography sx={{ fontSize: 22, fontWeight: 500, mb: 0.5 }}>
                Ask about your agent traces
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Answers are grounded in captured trace data — the assistant runs
                real queries, it doesn’t guess.
              </Typography>
              <Box
                sx={{
                  display: 'flex',
                  flexWrap: 'wrap',
                  gap: 1,
                  justifyContent: 'center',
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
            </Box>
          ) : (
            messages.map((m, i) => <Bubble key={i} message={m} />)
          )}

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

      {/* Composer */}
      <Box sx={{ borderTop: `1px solid ${BORDER_GRAY}`, bgcolor: '#fff' }}>
        <Box
          sx={{
            maxWidth: 760,
            mx: 'auto',
            px: 2,
            py: 1.5,
            display: 'flex',
            gap: 1,
            alignItems: 'flex-end',
          }}
        >
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
                send(input);
              }
            }}
            disabled={pending}
          />
          <IconButton
            color="primary"
            onClick={() => send(input)}
            disabled={pending || input.trim() === ''}
            aria-label="send"
            sx={{ mb: 0.25 }}
          >
            <SendIcon />
          </IconButton>
        </Box>
        <Typography
          variant="caption"
          sx={{ display: 'block', textAlign: 'center', color: TEXT_SECONDARY, pb: 1 }}
        >
          Enter to send · Shift+Enter for a new line
        </Typography>
      </Box>
    </Box>
  );
}

function Bubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === 'user';
  return (
    <Box
      sx={{
        display: 'flex',
        justifyContent: isUser ? 'flex-end' : 'flex-start',
        mb: 1.5,
      }}
    >
      <Paper
        sx={{
          px: 1.75,
          py: 1.25,
          maxWidth: '85%',
          bgcolor: isUser ? '#e8f0fe' : '#ffffff',
          border: `1px solid ${isUser ? '#d2e3fc' : BORDER_GRAY}`,
        }}
      >
        <Typography
          component="div"
          sx={{
            fontSize: 14,
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            color: '#202124',
          }}
        >
          {message.content}
        </Typography>
      </Paper>
    </Box>
  );
}
