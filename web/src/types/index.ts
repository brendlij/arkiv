export interface Asset {
  placeId: string;
  placeName: string;
  canManage: boolean;
  canAdd: boolean;
  canRemove: boolean;
  id: number;
  libraryId: string;
  relativePath: string;
  folder: string;
  filename: string;
  mediaType: "image" | "raw" | "video";
  mimeType: string;
  fileSize: number;
  takenAt: string;
  width: number;
  height: number;
  orientation: number;
  cameraMake: string;
  cameraModel: string;
  lens: string;
  focalLength: string;
  aperture: string;
  exposureTime: string;
  iso: string;
  latitude: number | null;
  longitude: number | null;
  duration: number;
  videoCodec: string;
  audioCodec: string;
  favorite: boolean;
  previewStatus: string;
  metadataStatus: string;
  error: string;
}
export interface User {
  id: number;
  username: string;
  displayName: string;
  role: "admin" | "member";
  enabled: boolean;
  mustChangePassword: boolean;
  proxy?: boolean;
}
export interface Album {
  ownerId: number;
  ownerName: string;
  role: "owner" | "viewer" | "contributor";
  description: string;
  id: number;
  name: string;
  count: number;
}
export interface Folder {
  libraryId: string;
  name: string;
  path: string;
  count: number;
}
export interface Stats {
  assets: number;
  images: number;
  videos: number;
  favorites: number;
  thumbnailQueue: number;
  failed: number;
  scanRunning: boolean;
  scanError: string;
  lastScan: string;
}
export interface Page {
  items: Asset[];
  nextCursor: string;
}
