/** Slice 5 fills this with the day's meals, grouped by the profile timezone. */
export default function TodayPage() {
  return (
    <main className="flex flex-1 flex-col gap-4 p-6">
      <h1>今日</h1>
      <p className="text-muted-foreground text-sm">
        還沒有紀錄。拍一張照片就完成一餐。
      </p>
    </main>
  );
}
