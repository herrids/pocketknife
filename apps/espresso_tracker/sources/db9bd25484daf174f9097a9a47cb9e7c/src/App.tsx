import { motion } from "framer-motion";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Toaster } from "@/components/ui/sonner";
import { Header } from "@/components/layout/header";
import { BeansView } from "@/components/beans/beans-view";
import { BrewsView } from "@/components/brews/brews-view";
import { useCrud } from "@/hooks/use-crud";
import { client } from "@/lib/client";
import type { Bean, BeanCreateInput, BeanUpdateInput, Brew, BrewCreateInput, BrewUpdateInput } from "@/client";

export default function App() {
  const beans = useCrud<Bean, BeanCreateInput, BeanUpdateInput>(
    {
      list: () => client.bean.list({ sort: ["-created_at"] }),
      create: (input) => client.bean.create(input),
      update: (id, input) => client.bean.update(id, input),
      delete: (id) => client.bean.delete(id),
    },
    "bean",
  );

  const brews = useCrud<Brew, BrewCreateInput, BrewUpdateInput>(
    {
      list: () => client.brew.list({ sort: ["-created_at"] }),
      create: (input) => client.brew.create(input),
      update: (id, input) => client.brew.update(id, input),
      delete: (id) => client.brew.delete(id),
    },
    "brew",
  );

  return (
    <div className="min-h-dvh bg-background text-foreground">
      <Header />
      <main className="mx-auto w-full max-w-3xl px-4 pb-16 pt-6 sm:px-6">
        <Tabs defaultValue="brews" className="w-full">
          <TabsList className="grid w-full grid-cols-2 sm:max-w-xs">
            <TabsTrigger value="brews">Brews</TabsTrigger>
            <TabsTrigger value="beans">Beans</TabsTrigger>
          </TabsList>

          <TabsContent value="brews" className="mt-6">
            <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.15 }}>
              <BrewsView brews={brews} beans={beans.rows} />
            </motion.div>
          </TabsContent>

          <TabsContent value="beans" className="mt-6">
            <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.15 }}>
              <BeansView beans={beans} />
            </motion.div>
          </TabsContent>
        </Tabs>
      </main>
      <Toaster richColors closeButton />
    </div>
  );
}
