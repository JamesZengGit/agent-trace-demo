'use client';

import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import TimelineIcon from '@mui/icons-material/Timeline';
import { GOOGLE_BLUE } from '@/lib/theme';

export default function AppHeader({ live }: { live?: boolean }) {
  return (
    <AppBar position="sticky">
      <Toolbar variant="dense" sx={{ minHeight: 56 }}>
        <TimelineIcon sx={{ color: GOOGLE_BLUE, mr: 1.5 }} />
        <Typography variant="h6" sx={{ fontSize: 20, mr: 1.5 }}>
          AgentTrace
        </Typography>
        <Typography variant="body2" color="text.secondary">
          network-layer agent observability
        </Typography>
        <Box sx={{ flex: 1 }} />
        {live !== undefined && (
          <Chip
            size="small"
            variant="outlined"
            label={live ? 'live' : 'reconnecting…'}
            sx={{
              color: live ? '#188038' : 'text.secondary',
              borderColor: live ? '#81c995' : 'divider',
              '& .MuiChip-label': { fontSize: 12 },
            }}
          />
        )}
      </Toolbar>
    </AppBar>
  );
}
