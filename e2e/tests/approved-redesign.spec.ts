import {test, expect, type Page} from '@playwright/test';

// Read-only presentation regressions use actual deployed pages and browser API
// mocks. The rest of the deployed suite exercises persistent product actions.
const initialSettings = {display_language:'en', timezone:'UTC', dashboard_order:[{type:'logging_gap',hidden:false},{type:'weight',hidden:false},{type:'steps',hidden:false}], birthdate: undefined, sex: undefined, preserve_me:'keep'};
const summary = {date:'2026-10-14',calories_consumed:1200,protein_grams_consumed:80,carbs_grams_consumed:130,fat_grams_consumed:40,dietary_fiber_grams_consumed:18.5,meal_count:3,last_logged_at:null,display_language:'en',target:{available:false,reason:'missing_goal_weight',dietary_fiber_grams:25}};
const meal = {id:'11111111-1111-4111-8111-111111111111',name:'Fiber meal',logged_at:'2026-10-13T12:00:00Z',status:'confirmed',calories:400,protein_grams:20,carbs_grams:30,fat_grams:10,dietary_fiber_grams:7.5};

async function mockPage(page:Page, meals:unknown[]=[meal]) {
  let settings: Record<string, unknown> = {...initialSettings};
  const writes: Record<string,unknown>[]=[];
  await page.route('**/api/**',async route=>{
    const request=route.request(),url=new URL(request.url());
    let body:unknown={};
    if(url.pathname==='/api/users/me')body={id:'22222222-2222-4222-8222-222222222222',username:'design-user',family_id:'33333333-3333-4333-8333-333333333333'};
    else if(url.pathname==='/api/users/me/settings'){
      if(request.method()==='PUT'){settings=request.postDataJSON();writes.push(settings)}
      body=settings;
    }
    else if(url.pathname==='/api/summary/today')body=summary;
    else if(url.pathname==='/api/data-types/presence')body={weight:true,steps:true};
    else if(url.pathname==='/api/food/meals')body=meals;
    else if(url.pathname.includes('needs-attention-count'))body={count:0};
    else if(url.pathname==='/api/food/meals/'+meal.id)body={...meal,items:[],photo_url:null};
    else if(url.pathname.includes('/api/data/')||url.pathname.includes('completeness')||url.pathname.includes('daily-totals'))body=[];
    else if(url.pathname.includes('weather'))body={hours:[],gaps:[]};
    await route.fulfill({json:body});
  });
  return writes;
}

test('mobile compact gear keeps inline eyes and ordering and independent fiber without calorie goal',async({page})=>{
  await page.setViewportSize({width:390,height:844});
  const writes=await mockPage(page);
  await page.goto('/');
  await expect(page.getByTestId('nutrition-today-fiber')).toContainText('18.5 / 25 g');
  await expect(page.getByTestId('log-food-links')).toBeHidden();
  const gear=page.getByRole('button',{name:'Customize',exact:true});
  const box=await gear.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(48);
  const heading=await page.getByRole('heading',{level:1}).boundingBox();
  expect(Math.abs((box?.y??0)-(heading?.y??0))).toBeLessThan(8);
  await gear.click();
  const grid=page.getByTestId('vitals-grid');
  await page.getByTestId('vital-card-weight-visibility').click();
  await expect(page.getByTestId('vital-card-weight')).toHaveAttribute('data-hidden','true');
  await page.getByRole('button',{name:'Move Weight up',exact:true}).click();
  await page.getByRole('button',{name:'Done',exact:true}).click();
  await expect(page.getByRole('button',{name:'Customize',exact:true})).toBeVisible();
  await expect(page.getByTestId('vital-card-weight')).toHaveCount(0);
  expect(writes.at(-1)?.preserve_me).toBe('keep');
  expect(writes.at(-1)?.dashboard_order).toEqual(expect.arrayContaining([{type:'weight',hidden:true}]));
  await page.getByRole('button',{name:'Customize',exact:true}).click();
  await expect(grid.locator('[data-testid]').first()).toBeAttached();
  await page.getByTestId('vital-card-weight-visibility').click();
  await page.getByRole('button',{name:'Done',exact:true}).click();
  await expect(page.getByTestId('vital-card-weight')).toBeVisible();
});

test('day fiber counts confirmed meals and excludes pending; missing fiber is unknown',async({page})=>{
  await mockPage(page,[meal,{...meal,id:'pending',status:'pending_review',dietary_fiber_grams:99}]);
  await page.goto('/food/history/');
  await expect(page.getByTestId('day-total-fiber')).toContainText('7.5g');
  await page.unroute('**/api/**');
  await mockPage(page,[{...meal,dietary_fiber_grams:null}]);
  await page.reload();
  await expect(page.getByTestId('day-total-fiber')).toContainText('—');
});

test('meal summary displays confirmed fiber and keeps pending aggregates unknown',async({page})=>{
  await mockPage(page);
  await page.goto('/food/review/?meal='+meal.id);
  await expect(page.getByTestId('meal-total-fiber')).toHaveText('7.5');
  await page.route('**/api/food/meals/'+meal.id,route=>route.fulfill({json:{...meal,status:'pending_review',items:[]}}));
  await page.reload();
  await expect(page.getByTestId('meal-total-fiber')).toHaveText('—');
});

test('fiber target saves and resets independently of an incomplete profile',async({page})=>{
  const writes=await mockPage(page);
  await page.goto('/settings/');
  await page.getByLabel('Fiber target, g/day (optional)',{exact:true}).fill('30');
  await page.getByTestId('save-fiber-target').click();
  await expect.poll(()=>writes.at(-1)?.fiber_target_grams).toBe(30);
  expect(writes.at(-1)?.preserve_me).toBe('keep');
  await page.getByLabel('Fiber target, g/day (optional)',{exact:true}).fill('');
  await page.getByTestId('save-fiber-target').click();
  await expect.poll(()=>writes.length).toBe(2);
  expect(writes.at(-1)).not.toHaveProperty('fiber_target_grams');
  await page.getByLabel('Fiber target, g/day (optional)',{exact:true}).fill('201');
  await page.getByTestId('save-fiber-target').click();
  await expect(page.getByText('Enter a whole-number fiber target between 1 and 200 g.')).toBeVisible();
  expect(writes.length).toBe(2);
});
