export type DeviceMode = 'desktop' | 'tablet' | 'mobile';

export interface BlockAttributes {
  [key: string]: any;
  content?: string;
  level?: 1 | 2 | 3 | 4 | 5 | 6;
  url?: string;
  alt?: string;
  caption?: string;
  align?: 'left' | 'center' | 'right' | 'justify';
  variant?: 'primary' | 'secondary' | 'outline' | 'ghost';
  size?: 'sm' | 'md' | 'lg';
  newTab?: boolean;
  columns?: number;
  gap?: string;
  padding?: string;
  margin?: string;
  backgroundColor?: string;
  textColor?: string;
  maxWidth?: string;
  width?: string;
  height?: string;
  style?: 'solid' | 'dashed' | 'dots';
  citation?: string;
  collectionUid?: string;
  limit?: number;
  layout?: 'grid' | 'list';
  showDate?: boolean;
  showExcerpt?: boolean;
  customClass?: string;
}

export interface Block {
  id: string;
  type: string;
  attributes: BlockAttributes;
  children?: Block[];
}

export interface PageSEO {
  meta_title?: string;
  meta_description?: string;
  canonical?: string;
  og_image?: string;
}

export interface PageDocument {
  id?: string;
  title: string;
  slug: string;
  template: 'default' | 'full-width' | 'landing';
  status: 'draft' | 'published';
  blocks: Block[];
  seo: PageSEO;
  draft_version?: number;
  published_version?: number;
  created_at?: string;
  updated_at?: string;
}

export type BlockCategory = 'text' | 'media' | 'layout' | 'dynamic';

export interface BlockDefinition {
  type: string;
  label: string;
  description: string;
  category: BlockCategory;
  icon: string;
  defaultAttributes: BlockAttributes;
  defaultChildren?: Block[];
}

