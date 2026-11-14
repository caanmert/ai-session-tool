import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import jsxA11y from "eslint-plugin-jsx-a11y";

// Containers may handle arrow keys for the buttons inside them (roving focus).
// Pointer handlers on non-interactive elements are still reported.
const pointerHandlers = { handlers: ["onClick", "onMouseDown", "onMouseUp"] };

export default tseslint.config(
  { ignores: ["dist", "wailsjs"] },
  js.configs.recommended,
  tseslint.configs.recommended,
  reactHooks.configs.flat.recommended,
  jsxA11y.flatConfigs.recommended,
  {
    files: ["src/**/*.{ts,tsx}"],
    languageOptions: { globals: globals.browser },
    rules: {
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_" },
      ],
      "jsx-a11y/no-static-element-interactions": ["error", pointerHandlers],
      "jsx-a11y/no-noninteractive-element-interactions": [
        "error",
        pointerHandlers,
      ],
      // These three rules prepare code for the React Compiler, which this app
      // does not use. Existing components set state at the start of effects
      // and keep "latest value" refs. Turn the rules on per component as each
      // is rebuilt.
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/refs": "off",
      "react-hooks/immutability": "off",
    },
  },
);
