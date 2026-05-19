import React from 'react';
import { cn } from './cn';

const buttonVariants = {
  primary: 'bg-blue-500 text-white hover:bg-blue-600',
  secondary: 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-white/10 dark:text-gray-300 dark:hover:bg-white/15',
  danger: 'bg-red-500 text-white hover:bg-red-600',
  warning: 'bg-amber-500 text-white hover:bg-amber-600',
  ghost: 'text-gray-500 hover:bg-gray-100 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-white/5 dark:hover:text-blue-200',
  subtle:
    'border border-gray-200 bg-white text-gray-700 hover:border-blue-300 hover:text-blue-600 dark:border-white/10 dark:bg-[#1C1C1E] dark:text-gray-200 dark:hover:border-blue-500/40 dark:hover:text-blue-200',
  dangerSubtle:
    'border border-red-200 bg-white text-red-500 hover:bg-red-50 dark:border-red-500/20 dark:bg-transparent dark:hover:bg-red-500/10 dark:hover:text-red-200',
};

const buttonSizes = {
  xs: 'rounded-lg px-2.5 py-1.5 text-xs',
  sm: 'rounded-xl px-3 py-2 text-xs',
  md: 'rounded-xl px-4 py-2 text-sm',
  lg: 'rounded-2xl px-5 py-2.5 text-sm',
};

export const Button = React.forwardRef(function Button({
  as: Component = 'button',
  children,
  className = '',
  variant = 'primary',
  size = 'md',
  fullWidth = false,
  type,
  ...props
}, ref) {
  return (
    <Component
      ref={ref}
      type={Component === 'button' ? type || 'button' : undefined}
      className={cn(
        'motion-button ui-button inline-flex items-center justify-center gap-2 font-bold outline-none focus-visible:ring-2 focus-visible:ring-blue-500/30 disabled:cursor-not-allowed disabled:opacity-50',
        buttonVariants[variant] || buttonVariants.primary,
        buttonSizes[size] || buttonSizes.md,
        fullWidth && 'w-full',
        className
      )}
      {...props}
    >
      {children}
    </Component>
  );
});
