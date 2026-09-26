// Shared between the library and player pages.

import { L } from "./i18n.ts";

/** A series is a name within an added folder. */
export const seriesKey = (folder: string, series: string): string => `${folder}\u0000${series}`;

/** "第 3 期 第 13 话" / "第3期 第13話", or the parts that are known. */
export function episodeLabel(season: number | null | undefined, episode: number | null | undefined, withSeason = true): string {
  const parts: string[] = [];
  if (withSeason && season != null) parts.push(L(`第 ${season} 期`, `第${season}期`));
  if (episode != null) parts.push(L(`第 ${episode} 话`, `第${episode}話`));
  return parts.join(" ");
}
