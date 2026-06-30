import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  darkMode: "media",
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Space Grotesk"', "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ['"Space Mono"', "ui-monospace", "monospace"],
      },
      colors: {
        // Light palette
        canvas: "#E8E0D0",
        surface: "#F2ECE0",
        card: "#FBF8F1",
        ink: "#1E1B15",
        "ink-muted": "#8A8071",
        "ink-faint": "#a99f8e",
        // Accent
        terracotta: "#DD6440",
        "terracotta-dark": "#C0512F",
        // App tile colors
        teal: "#3E9D93",
        purple: "#8E86CF",
        amber: "#E0A12E",
        "app-green": "#8FA968",
        "app-blue": "#5B9BD0",
        "app-pink": "#DD8AA6",
        "app-rust": "#B5544A",
      },
      borderRadius: {
        squircle: "19px",
        "squircle-lg": "24px",
        pill: "999px",
      },
      boxShadow: {
        tile: "0 3px 0 rgba(30,27,21,0.10)",
        nav: "0 16px 32px -10px rgba(30,27,21,0.55)",
        card: "inset 0 0 0 1.5px rgba(30,27,21,0.08)",
      },
      animation: {
        "pocket-pulse": "pocketPulse 1.8s ease-in-out infinite",
        "spring-in": "springIn 0.5s cubic-bezier(0.34,1.56,0.64,1)",
      },
      keyframes: {
        pocketPulse: {
          "0%,100%": { opacity: "0.45" },
          "50%": { opacity: "0.95" },
        },
        springIn: {
          "0%": { transform: "scale(0.93)", opacity: "0" },
          "100%": { transform: "scale(1)", opacity: "1" },
        },
      },
    },
  },
  plugins: [],
} satisfies Config;
