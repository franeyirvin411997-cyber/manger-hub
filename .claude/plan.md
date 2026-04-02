# Frontend Beautification Plan

## Current State
- Element Plus with default blue theme, inline styles everywhere
- Plain #409EFF header bar with just title text + logout button
- Light gray sidebar, basic `el-menu` with no branding
- Dashboard: flat stat cards in a row, no icons/colors per card
- All views: inconsistent page headers, no unified spacing/layout
- Login: minimal white card on gray background
- AI Chat: basic bordered div, no chat bubble styling
- `style.css` is leftover Vite scaffold boilerplate, unused

## Design Direction
Dark sidebar + clean white content area, modern admin dashboard style. Consistent spacing, better color usage, polished login page.

## Changes

### 1. `style.css` — Replace with app-level design tokens & utility classes
- CSS variables for sidebar, header, accent colors
- `.page-container` for unified view wrapper (padding, gap)
- `.page-header` for all views (title + actions row)
- `.stat-card` styles for Dashboard
- `.chat-bubble` styles for AI Chat
- Remove all Vite boilerplate (hero, counter, ticks, etc.)

### 2. `App.vue` — Dark sidebar + polished header
- Sidebar: dark background (#1d1e2c), white text, colored active item, platform logo/icon at top
- Header: white background with bottom border, breadcrumb-style page title on left, logout on right
- Collapse animation on sidebar (optional, stretch)

### 3. `Dashboard.vue` — Stat cards with color coding
- Each card gets a distinct accent color and icon
- Better typography: large number + label + sub-text layout
- Use `el-row` with responsive `el-col` `:xs/:sm/:md`

### 4. `Login.vue` — Modern login with gradient
- Full-page gradient background (dark blue to purple)
- Centered glass-morphism card with platform name and subtitle
- Styled input fields

### 5. `AIChat.vue` — Chat bubble UI
- User messages: right-aligned, blue bubble
- AI messages: left-aligned, gray bubble with subtle border
- Better input area with rounded corners

### 6. All other views — Consistent page structure
- Wrap content in `.page-container`
- Use `.page-header` pattern (h2 title + action buttons)
- Remove inline `style=""` attributes where possible, replace with CSS classes
- Tables: remove explicit `border` prop for cleaner look, use `stripe` instead

### Files to edit (14 files):
1. `web/src/style.css` — full rewrite
2. `web/src/App.vue` — sidebar + header redesign
3. `web/src/views/Dashboard.vue` — stat cards
4. `web/src/views/Login.vue` — gradient login
5. `web/src/views/AIChat.vue` — chat bubbles
6. `web/src/views/NodeList.vue` — consistent layout
7. `web/src/views/ProxyList.vue` — consistent layout
8. `web/src/views/GroupList.vue` — consistent layout
9. `web/src/views/BrowserList.vue` — consistent layout
10. `web/src/views/AccountList.vue` — consistent layout
11. `web/src/views/AppTemplateList.vue` — consistent layout
12. `web/src/views/DeployWizard.vue` — consistent layout
13. `web/src/views/SystemConfig.vue` — consistent layout
14. `web/src/views/RuleList.vue` — consistent layout
15. `web/src/views/OperationList.vue` — consistent layout
