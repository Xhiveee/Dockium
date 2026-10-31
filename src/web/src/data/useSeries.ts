import { useEffect, useState } from 'react'

export function useSeries(value: number, max = 40): number[] {
  const [values, setValues] = useState<number[]>([value])
  useEffect(() => {
    setValues((current) => {
      const next = [...current, value]
      return next.length > max ? next.slice(next.length - max) : next
    })
  }, [value, max])
  return values
}
