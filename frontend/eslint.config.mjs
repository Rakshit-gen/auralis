import next from "eslint-config-next";

const config = [
  ...(Array.isArray(next) ? next : [next]),
  {
    ignores: [".next/**", "node_modules/**"],
    rules: {
      "@next/next/no-img-element": "off",
      "react/no-unescaped-entities": "off",
      "react-hooks/set-state-in-effect": "off",
    },
  },
];

export default config;
