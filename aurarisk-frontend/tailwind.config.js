/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        risk: {
          normal:    { bg: "#dcfce7", text: "#166534", map: "#22c55e" },
          advisory:  { bg: "#fef9c3", text: "#854d0e", map: "#eab308" },
          alert:     { bg: "#ffedd5", text: "#9a3412", map: "#f97316" },
          emergency: { bg: "#fee2e2", text: "#991b1b", map: "#ef4444" },
        },
      },
    },
  },
  plugins: [],
};
