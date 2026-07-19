'use client';

// App-shell left rail. Links the two independent views — the dashboard
// (heatmap) and the chat — and highlights the active one. Mounted once in the
// root layout so it persists across routes.

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import TimelineIcon from '@mui/icons-material/Timeline';
import SpaceDashboardOutlinedIcon from '@mui/icons-material/SpaceDashboardOutlined';
import ChatBubbleOutlineOutlinedIcon from '@mui/icons-material/ChatBubbleOutlineOutlined';
import type { SvgIconComponent } from '@mui/icons-material';
import { GOOGLE_BLUE, BORDER_GRAY, TEXT_SECONDARY } from '@/lib/theme';

const WIDTH = 208;

interface Item {
  href: string;
  label: string;
  Icon: SvgIconComponent;
}

const items: Item[] = [
  { href: '/chat', label: 'Chat', Icon: ChatBubbleOutlineOutlinedIcon },
  { href: '/', label: 'Dashboard', Icon: SpaceDashboardOutlinedIcon },
];

export default function NavSidebar() {
  const pathname = usePathname();

  return (
    <Box
      component="nav"
      sx={{
        width: WIDTH,
        flexShrink: 0,
        position: 'sticky',
        top: 0,
        alignSelf: 'flex-start',
        height: '100vh',
        bgcolor: '#ffffff',
        borderRight: `1px solid ${BORDER_GRAY}`,
        display: 'flex',
        flexDirection: 'column',
        py: 1.5,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', px: 2, py: 1, mb: 1 }}>
        <TimelineIcon sx={{ color: GOOGLE_BLUE, mr: 1 }} />
        <Typography sx={{ fontSize: 18, fontWeight: 500 }}>AgentTrace</Typography>
      </Box>

      {items.map(({ href, label, Icon }) => {
        // Dashboard is active only at "/"; other entries match by prefix so
        // nested routes (e.g. /trace/:id) still highlight Dashboard.
        const active =
          href === '/' ? pathname === '/' || pathname.startsWith('/trace') : pathname.startsWith(href);
        return (
          <Box
            key={href}
            component={Link}
            href={href}
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1.5,
              mx: 1,
              px: 1.5,
              py: 1,
              borderRadius: 2,
              textDecoration: 'none',
              color: active ? GOOGLE_BLUE : '#202124',
              bgcolor: active ? '#e8f0fe' : 'transparent',
              fontWeight: active ? 600 : 400,
              '&:hover': { bgcolor: active ? '#e8f0fe' : '#f1f3f4' },
            }}
          >
            <Icon sx={{ fontSize: 20, color: active ? GOOGLE_BLUE : TEXT_SECONDARY }} />
            <Typography sx={{ fontSize: 14, fontWeight: 'inherit', color: 'inherit' }}>
              {label}
            </Typography>
          </Box>
        );
      })}
    </Box>
  );
}
