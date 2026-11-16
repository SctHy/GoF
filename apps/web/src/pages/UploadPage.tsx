import { ImagePlus, Upload, ArrowLeft } from "lucide-react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { PageMessage } from "@/components/PageMessage";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { errorMessage } from "../api/client.ts";
import type { FormEvent } from "react";
import type { PageProps } from "../types.ts";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/queryClient";
import { useEffect, useState } from "react";
import { apiRequest, uploadFile } from "../api/client.ts";

export function UploadPage({ session, onNavigate }: PageProps) {
  const client = useQueryClient();
  const [form, setForm] = useState({
    title: "",
    description: "",
  });
  const [videoFile, setVideoFile] = useState<File | null>(null);
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [coverPreview, setCoverPreview] = useState("");
  const [status, setStatus] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!coverFile) {
      setCoverPreview("");
      return;
    }
    const objectURL = URL.createObjectURL(coverFile);
    setCoverPreview(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [coverFile]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setStatus("");
    try {
      if (!videoFile) {
        throw new Error("请选择视频文件");
      }
      if (!coverFile) {
        throw new Error("请选择封面文件");
      }
      const [videoUpload, coverUpload] = await Promise.all([
        uploadFile(videoFile, "video", session.token),
        uploadFile(coverFile, "cover", session.token),
      ]);
      await apiRequest("/api/videos", {
        method: "POST",
        token: session.token,
        headers: {
          "Idempotency-Key": `web-upload-${Date.now()}`,
        },
        body: {
          title: form.title.trim(),
          description: form.description.trim(),
          media_url: videoUpload.url,
          cover_url: coverUpload.url,
        },
      });
      await client.invalidateQueries({ queryKey: queryKeys.ownVideos });
      setStatus("发布成功");
      onNavigate("/profile");
    } catch (error) {
      setStatus(errorMessage(error) || "发布失败");
    } finally {
      setSubmitting(false);
    }
  }

  if (!session.token)
    return (
      <div className="page-container">
        <PageMessage
          icon="lock"
          title="登录后发布视频"
          action="登录"
          onAction={() => onNavigate("/auth")}
        />
      </div>
    );
  return (
    <div className="page-container">
      <div className="page-heading">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">发布视频</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            上传视频和封面，填写标题后发布。
          </p>
        </div>
        <Button variant="outline" onClick={() => onNavigate("/profile")}>
          <ArrowLeft />
          我的作品
        </Button>
      </div>
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="shadow-none">
          <CardHeader>
            <CardTitle>视频信息</CardTitle>
            <CardDescription>
              选择视频和封面，再为作品写一段介绍。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-6" onSubmit={handleSubmit}>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="form-field">
                  <Label htmlFor="video-file">视频文件</Label>
                  <div className="space-y-2">
                    <Input
                      id="video-file"
                      aria-label="视频文件"
                      className="text-xs"
                      type="file"
                      accept="video/*"
                      onChange={(event) =>
                        setVideoFile(event.target.files?.[0] || null)
                      }
                    />
                  </div>
                </div>
                <div className="form-field">
                  <Label htmlFor="cover-file">视频封面</Label>
                  <div className="space-y-2">
                    <Input
                      id="cover-file"
                      aria-label="视频封面"
                      className="text-xs"
                      type="file"
                      accept="image/*"
                      onChange={(event) =>
                        setCoverFile(event.target.files?.[0] || null)
                      }
                    />
                  </div>
                </div>
              </div>
              <div className="form-field">
                <Label htmlFor="video-title">标题</Label>
                <Input
                  id="video-title"
                  value={form.title}
                  onChange={(event) =>
                    setForm({ ...form, title: event.target.value })
                  }
                  placeholder="输入视频标题"
                  required
                />
              </div>
              <div className="form-field">
                <Label htmlFor="video-description">
                  简介{" "}
                  <span className="font-normal text-muted-foreground">
                    选填
                  </span>
                </Label>
                <Textarea
                  id="video-description"
                  value={form.description}
                  onChange={(event) =>
                    setForm({ ...form, description: event.target.value })
                  }
                  placeholder="输入视频简介"
                  rows={5}
                />
              </div>
              {status && (
                <p role="status" className="text-sm text-destructive">
                  {status}
                </p>
              )}
              <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-5">
                <p className="text-xs text-muted-foreground">
                  发布后可在个人主页查看作品。
                </p>
                <Button type="submit" disabled={submitting}>
                  <Upload />
                  {submitting ? "发布中" : "发布"}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
        <Card className="gap-0 overflow-hidden py-0 shadow-none">
          <div className="flex aspect-3/4 items-center justify-center bg-muted">
            {coverPreview ? (
              <img
                src={coverPreview}
                alt="封面预览"
                className="size-full object-cover"
              />
            ) : (
              <div className="grid justify-items-center gap-3 text-muted-foreground">
                <ImagePlus className="size-9 stroke-1" />
                <p className="text-sm">封面预览</p>
              </div>
            )}
          </div>
          <CardContent className="space-y-3 p-5">
            <p className="text-xs text-muted-foreground">作品预览</p>
            <h2 className="font-medium wrap-anywhere">
              {form.title || "你的视频标题"}
            </h2>
            <p className="text-sm leading-relaxed text-muted-foreground wrap-anywhere">
              {form.description ||
                videoFile?.name ||
                "选择封面后，预览会显示在这里。"}
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
export default UploadPage;
