import { Suspense } from 'react';
import WeatherDayClient from './WeatherDayClient';
export default function WeatherDayPage() {
  return <Suspense><WeatherDayClient /></Suspense>;
}
