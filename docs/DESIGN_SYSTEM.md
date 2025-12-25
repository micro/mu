# Mu Design System

## Current State Analysis

### Existing UI Patterns

#### 1. **Card Component** (`.card`)
- Border: `1px solid #e0e0e0`
- Border radius: `5px`
- Padding: `20px`
- Background: `white` (implied)
- Margin bottom: `20px`
- **Used in**: Home page cards (news, blog, reminders)

#### 2. **Block Component** (`.block`)
- Border: `1px solid #f0f0f0`
- Border radius: `5px`
- Padding: `10px`
- Background: `#fafafa`
- **Used in**: Landing page feature blocks

#### 3. **Post Items** (`.post-item`)
- No border
- Padding bottom: `0`
- Hover: `background: #fafafa`
- **Used in**: Blog post listings

#### 4. **News Items** (`.news`, `.headline`)
- No border
- Minimal styling
- Margin bottom: `20px`
- **Used in**: News feed

#### 5. **Video Items**
- No card wrapper
- Just images with border-radius
- **Used in**: Video listings

#### 6. **Chat Messages** (`.message-item`)
- No border
- Hover: `background: #fafafa`
- **Used in**: Chat interface

#### 7. **Mail Thread Preview** (`.thread-preview`)
- Border bottom: `1px solid #eee`
- Padding: `15px`
- Hover: `background: #f5f5f5`
- **Used in**: Mail inbox/sent views

#### 8. **Mail Thread Messages** (`.thread-message`)
- Border: `1px solid #e0e0e0`
- Border radius: `5px`
- Padding: `20px`
- Background: `white`
- **Used in**: Mail thread view

### Issues Identified

1. **Inconsistent containment**: Some items use cards, others don't
2. **Varying hover states**: `#fafafa` vs `#f5f5f5` vs none
3. **Border inconsistency**: Some use borders, others use bottom borders only
4. **Spacing variations**: Different padding/margin values across components
5. **No visual hierarchy**: Hard to distinguish between different content types when scanning

## Proposed Unified Design System

### Core Principles

1. **Consistent card-based layout** for all content items
2. **Clear visual hierarchy** through consistent spacing and borders
3. **Subtle hover states** for interactive elements
4. **Responsive and accessible** design patterns

### Design Tokens

#### Colors
```css
--card-border: #e0e0e0
--card-background: #ffffff
--hover-background: #f5f5f5
--divider: #eee
--text-primary: #333
--text-secondary: #666
--text-muted: #999
--accent-blue: #007bff
```

#### Spacing
```css
--spacing-xs: 8px
--spacing-sm: 12px
--spacing-md: 16px
--spacing-lg: 20px
--spacing-xl: 24px
```

#### Borders
```css
--border-radius: 5px
--border-width: 1px
```

### Component Standards

#### 1. **Content Card** (Unified)
All content items (blog posts, news, videos, chat messages) should use:
- Border: `1px solid var(--card-border)`
- Border radius: `var(--border-radius)`
- Padding: `var(--spacing-lg)`
- Background: `var(--card-background)`
- Margin bottom: `var(--spacing-md)`
- Hover: `background: var(--hover-background)`
- Transition: `background 0.2s`

#### 2. **List Item** (For compact lists)
Mail previews, compact listings should use:
- Border bottom: `1px solid var(--divider)`
- Padding: `var(--spacing-md)`
- Hover: `background: var(--hover-background)`
- Transition: `background 0.2s`

#### 3. **Interactive Elements**
- Buttons, links, clickable cards should have clear hover states
- Consistent transition timing: `0.2s`
- Clear focus states for accessibility

## Implementation Plan

### Phase 1: CSS Refactoring
1. Create CSS variables for design tokens
2. Create unified card component classes
3. Create list item component classes

### Phase 2: Component Updates
1. Update blog posts to use content cards
2. Update news items to use content cards
3. Update video items to use content cards
4. Update chat messages to use content cards (or list items for compact view)
5. Ensure mail components align with system

### Phase 3: Consistency Check
1. Review all pages for visual consistency
2. Test hover states across all components
3. Verify mobile responsiveness
4. Accessibility audit

## Benefits

1. **Visual Cohesion**: Users experience consistent UI across all pages
2. **Easier Maintenance**: Single source of truth for styling
3. **Better UX**: Predictable interactions and visual patterns
4. **Faster Development**: Reusable components speed up new feature development
