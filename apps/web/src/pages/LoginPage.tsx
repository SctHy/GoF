import { ArrowLeft, ArrowRight } from "lucide-react";
import { Brand } from "@/components/Brand";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { errorMessage } from "../api/client.ts";
import type { User } from "../types.ts";
import type { FormEvent } from "react";
import type { PageProps } from "../types.ts";
import { useState } from "react";
import { apiData, apiRequest } from "../api/client.ts";

export function LoginPage({ session, onNavigate }: PageProps) {
  const [mode, setMode] = useState("login");
  const [form, setForm] = useState({ account: "", password: "", nickname: "" });
  const [message, setMessage] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setMessage("");
    try {
      if (mode === "register") {
        await apiRequest("/api/users", {
          method: "POST",
          body: {
            account: form.account.trim(),
            password: form.password,
            nickname: form.nickname.trim(),
          },
        });
      }
      const tokenResponse = await apiData<{ access_token: string }>(
        "/api/sessions",
        {
          method: "POST",
          body: {
            account: form.account.trim(),
            password: form.password,
          },
        },
      );
      const accessToken = tokenResponse.access_token;
      const profile = await apiData<User>("/api/users/me", {
        token: accessToken,
      });
      session.setAuth(accessToken, profile);
      onNavigate("/recommend");
    } catch (error) {
      setMessage(errorMessage(error) || "登录失败，请检查账号与密码");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex min-h-dvh flex-col bg-sidebar">
      <header className="flex items-center justify-between p-5 md:px-10 md:py-7">
        <div className="flex items-center gap-2.5">
          <Brand />
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onNavigate("/timeline")}
        >
          <ArrowLeft />
          返回浏览
        </Button>
      </header>
      <div className="flex flex-1 items-center justify-center px-5 py-10">
        <div className="w-full max-w-sm space-y-6">
          <Card className="gap-6 py-7 shadow-sm">
            <CardHeader className="space-y-2 text-center">
              <CardTitle
                role="heading"
                aria-level={1}
                className="text-2xl tracking-tight"
              >
                {mode === "login" ? "欢迎回来" : "加入 GCFeed"}
              </CardTitle>
              <CardDescription>
                {mode === "login"
                  ? "登录账号，继续发现你喜欢的内容。"
                  : "创建账号，分享你的精彩瞬间。"}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-5" onSubmit={handleSubmit}>
                <Tabs
                  value={mode}
                  onValueChange={(value) => {
                    if (value === "login" || value === "register") {
                      setMode(value);
                      setMessage("");
                    }
                  }}
                >
                  <TabsList className="w-full" aria-label="登录方式">
                    <TabsTrigger className="flex-1" value="login">
                      登录
                    </TabsTrigger>
                    <TabsTrigger className="flex-1" value="register">
                      注册
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
                <div className="form-field">
                  <Label htmlFor="auth-account">账号</Label>
                  <Input
                    id="auth-account"
                    value={form.account}
                    onChange={(event) =>
                      setForm({ ...form, account: event.target.value })
                    }
                    placeholder="请输入账号"
                    autoComplete="username"
                    required
                  />
                </div>
                {mode === "register" && (
                  <div className="form-field">
                    <Label htmlFor="auth-name">昵称</Label>
                    <Input
                      id="auth-name"
                      value={form.nickname}
                      onChange={(event) =>
                        setForm({ ...form, nickname: event.target.value })
                      }
                      placeholder="输入昵称"
                      autoComplete="nickname"
                      required
                    />
                  </div>
                )}
                <div className="form-field">
                  <Label htmlFor="auth-password">密码</Label>
                  <Input
                    id="auth-password"
                    value={form.password}
                    onChange={(event) =>
                      setForm({ ...form, password: event.target.value })
                    }
                    placeholder="输入密码"
                    type="password"
                    autoComplete={
                      mode === "register" ? "new-password" : "current-password"
                    }
                    required
                  />
                </div>
                {message && (
                  <p
                    role="alert"
                    className="rounded-lg bg-destructive/5 p-3 text-sm text-destructive"
                  >
                    {message}
                  </p>
                )}
                <Button
                  type="submit"
                  size="lg"
                  className="w-full"
                  disabled={submitting}
                >
                  {submitting
                    ? "提交中"
                    : mode === "register"
                      ? "注册并登录"
                      : "登录"}
                  <ArrowRight />
                </Button>
              </form>
            </CardContent>
          </Card>
          <p className="text-center text-xs leading-relaxed text-muted-foreground">
            看见生活里的灵感，也让你的作品被看见。
          </p>
        </div>
      </div>
    </main>
  );
}
export default LoginPage;
