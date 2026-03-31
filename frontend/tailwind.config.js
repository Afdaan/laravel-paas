/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Catppuccin Base overrides for light/dark swapping
        white: '#eff1f5', // Latte Base (light mode bg, dark mode text)
        black: '#11111b', // Mocha Crust (deepest dark)
        
        // Mapped to Catppuccin to seamlessly swap between Latte and Mocha
        slate: {
          50: '#e6e9ef',  // Latte Mantle (light card bg)
          100: '#dce0e8', // Latte Crust (light alternate bg)
          200: '#ccd0da', // Latte Surface0 (light border)
          300: '#bcc0cc', // Latte Surface1 (light border hover)
          400: '#a6adc8', // Mocha Subtext0 (dark muted text)
          500: '#7f849c', // Mocha Overlay0 (neutral)
          600: '#5c5f77', // Latte Subtext0 (light muted text)
          700: '#313244', // Mocha Surface0 (dark alternate bg / border)
          800: '#181825', // Mocha Mantle (dark elevated card bg)
          900: '#1e1e2e', // Mocha Base (dark bg, light text)
          950: '#11111b', // Mocha Crust (deepest dark bg)
        },
        
        // Accents (Catppuccin Mocha mapped)
        indigo: {
          400: '#b4befe', // Lavender
          500: '#89b4fa', // Blue
          600: '#74c7ec', // Sapphire
        },
        emerald: {
          400: '#a6e3a1', // Green
          500: '#94e2d5', // Teal
        },
        amber: {
          400: '#f9e2af', // Yellow
          500: '#fab387', // Peach
        },
        rose: {
          400: '#f38ba8', // Red
          500: '#eba0ac', // Maroon
          600: '#e64553', // Red darker
        },
        blue: {
          400: '#8caaee', // Light blue
          500: '#89b4fa', // Blue
        },
        
        // Keep original primary for legacy compatibility where required
        primary: {
          50: '#f0f9ff',
          100: '#e0f2fe',
          200: '#bae6fd',
          300: '#7dd3fc',
          400: '#89b4fa',
          500: '#74c7ec',
          600: '#89dceb',
          700: '#0369a1',
          800: '#075985',
          900: '#0c4a6e',
        },
      },
    },
  },
  plugins: [],
}
