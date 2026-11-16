import { feedResponse } from "./fixtures.ts";
import { test } from 'vitest'
import assert from "node:assert/strict";
import { appendFeedItems, mapFeedItem, paginationErrorMessage } from "../src/lib/feed.ts";
import { normalizePlaybackConfig } from "../src/lib/playback.ts";

test("overlapping Feed pages retain stable order without mutating existing items", () => {
  const original = [{video_id:3}, {video_id:2}];
  const merged = appendFeedItems(original, [{video_id:2}, {video_id:1}, {video_id:1}]);
  assert.deepEqual(merged.map((item) => item.video_id), [3,2,1]);
  assert.equal(original.length, 2);
  assert.equal(merged[0], original[0]);
  assert.equal(appendFeedItems(original, []), original);
});

test("Feed mapper carries viewer actions and request identity", () => {
  const item = mapFeedItem({...feedResponse, video_id:4, author_id:2, liked:true, favorited:false}, "recommend", "request-1");
  assert.equal(item.liked, true);
  assert.equal(item.favorited, false);
  assert.equal(item.feed_scene, "recommend");
  assert.equal(item.request_id, "request-1");
});

test("invalid remote preload configuration stays bounded", () => {
  assert.equal(normalizePlaybackConfig({preload_count:1000}).preload_count, 10);
  assert.equal(normalizePlaybackConfig({preload_count:0}).preload_count, 1);
  assert.equal(normalizePlaybackConfig({buffer_ms:-1}).buffer_ms, 1200);
});

test("expired snapshot errors get a refresh explanation, other failures stay generic", () => {
  assert.equal(paginationErrorMessage({status:400, message:"cursor is invalid"}), "分页已过期，请刷新");
  assert.equal(paginationErrorMessage({status:500, message:"private server error"}), "加载更多失败，请刷新重试");
});
