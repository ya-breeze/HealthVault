import { describe, expect, it } from 'vitest';
import { dayWeather, latestWeather, validateWeatherHistory, validWeatherDate, WEATHER_UNITS, weatherDayBounds, weatherQueryBounds, weatherTimezone } from './weather';
import type { WeatherHistory, WeatherHour } from './api';
function hour(time:string,temperature=0):WeatherHour {return {hour:time,temperature_c:temperature,apparent_temperature_c:1,relative_humidity_percent:60,surface_pressure_hpa:1000,mean_sea_level_pressure_hpa:1020,precipitation_mm:0,wind_speed_kmh:2,source:'open-meteo historical-forecast',model:'ecmwf_ifs025',latitude:50.1,longitude:14.4,fetched_at:time,first_observation_id:'a',last_observation_id:'b'};}
function history(hours:WeatherHour[]=[]):WeatherHistory {return {hours,gaps:[],units:{...WEATHER_UNITS}};}
describe('weather day and evidence helpers',()=>{
 it('selects latest saved hour without relying on response order, keeping genuine zero',()=>{
  expect(latestWeather(history([hour('2026-10-04T02:00:00Z',1),hour('2026-10-04T03:00:00Z',0),hour('2026-10-04T01:00:00Z',4)]))?.temperature_c).toBe(0);
  expect(latestWeather(history())).toBeNull();
 });
 it('rejects old, future and unfinished hours for the seven-day latest query',()=>{
  const now=new Date('2026-10-04T03:30:00Z');
  expect(latestWeather(history([hour('2026-10-04T03:00:00Z'),hour('2026-10-05T03:00:00Z'),hour('2026-09-26T01:00:00Z'),hour('2026-10-04T02:00:00Z')]),now)?.hour).toBe('2026-10-04T02:00:00Z');
 });
 it('resolves DST days as 23 and 25 hours and falls back for invalid zones',()=>{
  for(const [date,hours] of [['2026-03-29',23],['2026-10-25',25]] as const){const r=weatherDayBounds(date,'Europe/Prague');expect((r.to-r.from)/3600000).toBe(hours);}
  expect(weatherTimezone('nonsense')).toBe('UTC');
  expect(weatherDayBounds('2026-10-04','nonsense').from).toBe(Date.parse('2026-10-04T00:00:00Z'));
 });
 it('uses actual Kathmandu midnight and rounds query outwards without shifting sample timestamps',()=>{
  const bounds=weatherQueryBounds('2026-10-04','Asia/Kathmandu',new Date('2026-10-06T00:00:00Z'));
  expect(new Date(bounds.day.from).toISOString()).toBe('2026-10-03T18:15:00.000Z');
  expect(bounds.from).toBe('2026-10-03T18:00:00.000Z');
  expect(bounds.to).toBe('2026-10-04T19:00:00.000Z');
  const points=dayWeather(history([hour('2026-10-03T18:00:00Z',9)]),bounds.day.from,bounds.day.to).points;
  expect(points[0].time).toBe(Date.parse('2026-10-03T18:00:00Z'));
 });
 it('clamps current day at now and does not request future days',()=>{
  const now=new Date('2026-10-04T03:30:00Z');
  expect(weatherQueryBounds('2026-10-04','UTC',now).to).toBe(now.toISOString());
  expect(weatherQueryBounds('2026-10-05','UTC',now).elapsed).toBe(false);
 });
 it('rejects invalid dates and omitted calendar days',()=>{
  expect(validWeatherDate('2026-02-31')).toBe(false);expect(validWeatherDate('2026-02-28')).toBe(true);
  expect(()=>weatherDayBounds('2011-12-30','Pacific/Apia')).toThrow();
 });
 it('inserts null hours and clips gaps without fabricating zero weather',()=>{
  const data=history([hour('2026-10-04T02:00:00Z',0),hour('2026-10-04T04:00:00Z',5)]);
  data.gaps=[{from:'2026-10-04T00:15:00Z',to:'2026-10-04T03:30:00Z',reason:'moving'}];
  const out=dayWeather(data,Date.parse('2026-10-04T02:00:00Z'),Date.parse('2026-10-04T05:00:00Z'));
  expect(out.points.map(p=>p.temperature)).toEqual([0,null,5]);
  expect(out.gaps[0].from).toBe('2026-10-04T02:00:00.000Z');
 });
 it('rejects malformed values, duplicate hours and wrong units',()=>{
  const data=history([hour('2026-10-04T02:00:00Z')]);expect(validateWeatherHistory(data)).toBe(data);
  expect(()=>validateWeatherHistory({...data,units:{}})).toThrow();
  expect(()=>validateWeatherHistory(history([hour('2026-10-04T02:00:00Z',NaN)]))).toThrow();
  expect(()=>validateWeatherHistory(history([data.hours[0],data.hours[0]]))).toThrow();
 });
});


describe('weather provenance validation', () => {
  it.each([
    {latitude:null}, {latitude:undefined}, {latitude:'50.1'}, {latitude:NaN}, {latitude:91}, {latitude:-91},
    {longitude:null}, {longitude:undefined}, {longitude:'14.4'}, {longitude:Infinity}, {longitude:181}, {longitude:-181},
    {fetched_at:null}, {fetched_at:undefined}, {fetched_at:123}, {fetched_at:'not-a-time'},
  ])('rejects malformed provenance %j before rendering', patch => {
    const malformed = JSON.parse(JSON.stringify({...hour('2026-10-04T02:00:00Z'), ...patch}));
    expect(()=>validateWeatherHistory(history([malformed]))).toThrow('Invalid weather hour');
  });
  it('retains valid coordinate boundary and zero values', () => {
    for(const [latitude, longitude] of [[0,0],[-90,-180],[90,180]]) {
      const value = history([{...hour('2026-10-04T02:00:00Z'),latitude,longitude}]);
      expect(validateWeatherHistory(value)).toBe(value);
    }
  });
});
