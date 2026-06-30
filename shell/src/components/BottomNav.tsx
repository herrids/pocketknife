import { useLocation, useNavigate } from "react-router-dom";

const NAV_ITEMS = [
  { icon: "⌂", label: "Home", path: "/home" },
  { icon: "⌕", label: "Search", path: "/search" },
  { icon: "◷", label: "Recent", path: "/recent" },
  { icon: "☰", label: "Menu", path: "/menu" },
] as const;

export function BottomNav() {
  const navigate = useNavigate();
  const location = useLocation();

  return (
    <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50">
      <div
        className="flex items-center gap-1 px-2.5 py-2 rounded-[24px] bg-ink dark:bg-[#0E0C09]"
        style={{ boxShadow: "0 16px 32px -10px rgba(30,27,21,0.55)" }}
      >
        {NAV_ITEMS.map((item) => {
          const active = location.pathname === item.path;
          return (
            <button
              key={item.path}
              onClick={() => navigate(item.path)}
              className={`
                w-[46px] h-10 flex items-center justify-center rounded-[15px] text-lg
                transition-colors duration-100 press-spring-sm
                ${active
                  ? "bg-terracotta text-white"
                  : "text-[#8A8276] dark:text-[#6f6a60] hover:text-ink-muted dark:hover:text-[#9c9282]"
                }
              `}
              aria-label={item.label}
            >
              {item.icon}
            </button>
          );
        })}
      </div>
    </div>
  );
}
