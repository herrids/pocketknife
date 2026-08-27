import { useState } from "react";
import { Sun, Moon } from "lucide-react";
import { Toaster } from "@/components/ui/sonner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/lib/use-theme";
import { BabyView } from "@/components/baby/BabyView";
import { SprungView } from "@/components/sprung/SprungView";
import { FaehigkeitView } from "@/components/faehigkeit/FaehigkeitView";
import { ErreichungView } from "@/components/erreichung/ErreichungView";
import { TagebuchView } from "@/components/tagebuch/TagebuchView";

export default function App() {
  const { theme, toggle } = useTheme();
  const [activeTab, setActiveTab] = useState("baby");

  return (
    <div className="min-h-dvh bg-background text-foreground">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="mx-auto flex h-14 max-w-2xl items-center justify-between px-4">
          <div className="flex items-center gap-2">
            <span className="text-xl leading-none">🍼</span>
            <h1 className="text-base font-semibold tracking-tight">Babyentwicklung</h1>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={toggle}
            aria-label={theme === "dark" ? "Helles Design aktivieren" : "Dunkles Design aktivieren"}
          >
            {theme === "dark" ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
          </Button>
        </div>
      </header>

      <main className="mx-auto max-w-2xl">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <div className="sticky top-14 z-30 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 px-4 py-2">
            <TabsList className="grid w-full grid-cols-5 h-auto">
              <TabsTrigger value="baby" className="text-xs py-1.5">
                Baby
              </TabsTrigger>
              <TabsTrigger value="spruenge" className="text-xs py-1.5">
                Sprünge
              </TabsTrigger>
              <TabsTrigger value="faehigkeiten" className="text-xs py-1.5">
                Skills
              </TabsTrigger>
              <TabsTrigger value="erfolge" className="text-xs py-1.5">
                Erfolge
              </TabsTrigger>
              <TabsTrigger value="tagebuch" className="text-xs py-1.5">
                Tagebuch
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent value="baby" className="mt-0 focus-visible:ring-0">
            <BabyView />
          </TabsContent>
          <TabsContent value="spruenge" className="mt-0 focus-visible:ring-0">
            <SprungView />
          </TabsContent>
          <TabsContent value="faehigkeiten" className="mt-0 focus-visible:ring-0">
            <FaehigkeitView />
          </TabsContent>
          <TabsContent value="erfolge" className="mt-0 focus-visible:ring-0">
            <ErreichungView />
          </TabsContent>
          <TabsContent value="tagebuch" className="mt-0 focus-visible:ring-0">
            <TagebuchView />
          </TabsContent>
        </Tabs>
      </main>

      <Toaster richColors closeButton />
    </div>
  );
}
