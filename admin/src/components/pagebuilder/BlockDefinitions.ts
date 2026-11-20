import type { BlockDefinition } from './types';

export const BLOCK_DEFINITIONS: BlockDefinition[] = [
  // Text Category
  {
    type: 'core/heading',
    label: 'Heading',
    description: 'Introduce sections and structure your page content.',
    category: 'text',
    icon: 'Heading',
    defaultAttributes: {
      level: 2,
      content: 'Heading Text',
      align: 'left',
      textColor: '',
    },
  },
  {
    type: 'core/paragraph',
    label: 'Paragraph',
    description: 'Standard rich text paragraph for body copy and narratives.',
    category: 'text',
    icon: 'Type',
    defaultAttributes: {
      content: 'Write your story or description here...',
      align: 'left',
      textColor: '',
    },
  },
  {
    type: 'core/quote',
    label: 'Quote',
    description: 'Call out key testimonials, quotes, or noteworthy phrases.',
    category: 'text',
    icon: 'Quote',
    defaultAttributes: {
      content: 'Design is not just what it looks like and feels like. Design is how it works.',
      citation: 'Steve Jobs',
    },
  },

  // Media Category
  {
    type: 'core/image',
    label: 'Image',
    description: 'Display an image with optional caption, alignment, and sizing.',
    category: 'media',
    icon: 'Image',
    defaultAttributes: {
      url: 'https://images.unsplash.com/photo-1498050108023-c5249f4df085?auto=format&fit=crop&w=1200&q=80',
      alt: 'Laptop and workspace',
      caption: 'High-performance Go CMS block editing',
      align: 'center',
    },
  },

  // Layout Category
  {
    type: 'core/section',
    label: 'Section',
    description: 'Full-width wrapper with background color, padding, and layout controls.',
    category: 'layout',
    icon: 'Layout',
    defaultAttributes: {
      padding: 'py-12',
      backgroundColor: '#f8fafc',
      textColor: '#0f172a',
      customClass: '',
    },
    defaultChildren: [
      {
        id: 'inner-cont',
        type: 'core/container',
        attributes: { maxWidth: 'max-w-5xl', padding: 'px-4' },
        children: [
          {
            id: 'inner-h2',
            type: 'core/heading',
            attributes: { level: 2, content: 'Section Title', align: 'center' },
          },
          {
            id: 'inner-p',
            type: 'core/paragraph',
            attributes: { content: 'Add content inside this section container.', align: 'center' },
          },
        ],
      },
    ],
  },
  {
    type: 'core/container',
    label: 'Container',
    description: 'Constrain and center content horizontally with customizable max width.',
    category: 'layout',
    icon: 'Box',
    defaultAttributes: {
      maxWidth: 'max-w-5xl',
      padding: 'px-4',
    },
    defaultChildren: [],
  },
  {
    type: 'core/columns',
    label: 'Columns',
    description: 'Organize content into a multi-column responsive grid layout.',
    category: 'layout',
    icon: 'Columns3',
    defaultAttributes: {
      columns: 2,
      gap: 'gap-6',
    },
    defaultChildren: [
      {
        id: 'col-1',
        type: 'core/column',
        attributes: { width: 'w-full' },
        children: [
          {
            id: 'col-1-h',
            type: 'core/heading',
            attributes: { level: 3, content: 'Column One' },
          },
          {
            id: 'col-1-p',
            type: 'core/paragraph',
            attributes: { content: 'First column content details.' },
          },
        ],
      },
      {
        id: 'col-2',
        type: 'core/column',
        attributes: { width: 'w-full' },
        children: [
          {
            id: 'col-2-h',
            type: 'core/heading',
            attributes: { level: 3, content: 'Column Two' },
          },
          {
            id: 'col-2-p',
            type: 'core/paragraph',
            attributes: { content: 'Second column content details.' },
          },
        ],
      },
    ],
  },
  {
    type: 'core/separator',
    label: 'Separator',
    description: 'Add a visual dividing line between distinct sections.',
    category: 'layout',
    icon: 'Minus',
    defaultAttributes: {
      style: 'solid',
    },
  },
  {
    type: 'core/spacer',
    label: 'Spacer',
    description: 'Add empty vertical whitespace to breathe room between blocks.',
    category: 'layout',
    icon: 'MoveVertical',
    defaultAttributes: {
      height: '32px',
    },
  },

  // Actions Category
  {
    type: 'core/button',
    label: 'Button',
    description: 'Call-to-action button linking to internal pages or external websites.',
    category: 'text',
    icon: 'MousePointerClick',
    defaultAttributes: {
      text: 'Explore More',
      url: '#',
      variant: 'primary',
      newTab: false,
      align: 'left',
    },
  },

  // Dynamic CMS Category
  {
    type: 'core/dynamic-posts',
    label: 'CMS Posts List',
    description: 'Dynamically query and display live content from any CMS collection.',
    category: 'dynamic',
    icon: 'Database',
    defaultAttributes: {
      collectionUid: 'blog-posts',
      limit: 6,
      layout: 'grid',
      showDate: true,
      showExcerpt: true,
    },
  },
];

