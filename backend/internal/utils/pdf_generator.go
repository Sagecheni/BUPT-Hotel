// internal/utils/pdf_generator.go

package utils

import (
	"backend/internal/db"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// Bill 账单结构体
type Bill struct {
	RoomID       int
	ClientName   string
	ClientID     string
	CheckInTime  time.Time
	CheckOutTime time.Time
	DaysStayed   int
	RoomRate     float32 // 每日房费
	TotalRoom    float32 // 住宿总费用
	TotalAC      float32 // 空调总费用
	Deposit      float32 // 押金
	FinalTotal   float32 // 最终总费用（住宿费+空调费-押金）
}

type DetailBill struct {
	RoomID       int
	ClientName   string
	ClientID     string
	CheckInTime  time.Time
	CheckOutTime time.Time
	TotalCost    float32
	Details      []db.Detail
}

var detailTypeMap = map[db.DetailType]string{
	db.DetailTypeServiceStart:     "服务开始",
	db.DetailTypeServiceInterrupt: "服务结束",
	db.DetailTypeSpeedChange:      "调整风速",
	db.DetailTypeTargetReached:    "达到目标温度",
	db.DetailTypeTemp:             "调整温度",
}

func GenerateDetailPDF(bill DetailBill) (*gofpdf.Fpdf, error) {
	// 使用横向A4纸，并设置页边距
	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.AddPage()

	// 添加中文字体
	pdf.AddUTF8Font("chinese", "", "./SimHei.ttf")

	// 设置标题
	pdf.SetFont("chinese", "", 18)
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(280, 15, "波普特酒店 - 空调使用详单")
	pdf.Ln(20)

	// 添加分隔线
	pdf.Line(10, pdf.GetY(), 280, pdf.GetY())
	pdf.Ln(5)

	// 基本信息部分
	pdf.SetFont("chinese", "", 11)
	drawInfoSection(pdf, bill)

	// 添加分隔线
	pdf.Ln(5)
	pdf.Line(10, pdf.GetY(), 280, pdf.GetY())
	pdf.Ln(5)

	// 表格部分（预留页脚空间）
	drawDetailTable(pdf, bill)

	// 总计信息
	pdf.Ln(5)
	pdf.SetFont("chinese", "", 12)
	pdf.SetTextColor(0, 102, 204)
	pdf.Cell(220, 10, "总费用:")
	pdf.Cell(40, 10, fmt.Sprintf("%.2f元", bill.TotalCost))

	// 在最后一页添加页脚
	footerHeight := 15.0 // 页脚高度
	bottomMargin := 10.0 // 底部边距
	pageHeight := 210.0  // A4纸横向高度

	// 确保页脚位置正确
	remainingHeight := pageHeight - pdf.GetY() - footerHeight - bottomMargin
	if remainingHeight > 0 {
		pdf.Ln(remainingHeight)
	}

	// 绘制页脚
	drawFooter(pdf)

	return pdf, nil
}

func drawInfoSection(pdf *gofpdf.Fpdf, bill DetailBill) {
	// 第一行
	pdf.Cell(20, 8, "房间号:")
	pdf.SetTextColor(0, 102, 204)
	pdf.Cell(30, 8, fmt.Sprintf("%d", bill.RoomID))
	pdf.SetTextColor(0, 0, 0)

	pdf.Cell(25, 8, "客户姓名:")
	pdf.Cell(60, 8, bill.ClientName)

	pdf.Cell(25, 8, "身份证号:")
	pdf.Cell(120, 8, bill.ClientID)
	pdf.Ln(10)

	// 第二行
	pdf.Cell(20, 8, "入住时间:")
	pdf.Cell(100, 8, bill.CheckInTime.Format("2006-01-02 15:04:05"))

	pdf.Cell(20, 8, "退房时间:")
	pdf.Cell(100, 8, bill.CheckOutTime.Format("2006-01-02 15:04:05"))
	pdf.Ln(10)
}

func drawDetailTable(pdf *gofpdf.Fpdf, bill DetailBill) {
	// 设置表头
	headers := []struct {
		width float64
		name  string
	}{
		{25, "房间号"},
		{35, "请求时间"},
		{35, "开始时间"},
		{35, "结束时间"},
		{25, "服务时长"},
		{20, "风速"},
		{25, "费率"},
		{25, "当前温度"},
		{25, "目标温度"},
		{20, "费用"},
		{30, "操作类型"},
	}

	// 设置表头样式
	pdf.SetFont("chinese", "", 10)
	pdf.SetFillColor(240, 240, 240)
	pdf.SetTextColor(0, 0, 0)

	// 绘制表头
	for _, h := range headers {
		pdf.Cell(h.width, 10, h.name)
	}
	pdf.Ln(10)

	// 设置表格内容字体
	pdf.SetFont("chinese", "", 9)
	var fill bool = false

	// 计算每行的高度
	rowHeight := 8.0

	for _, detail := range bill.Details {
		// 检查是否需要新页
		if pdf.GetY() > 180 { // 留出足够空间给页脚
			pdf.AddPage()
			// 重新绘制表头
			pdf.SetFont("chinese", "", 10)
			for _, h := range headers {
				pdf.Cell(h.width, 10, h.name)
			}
			pdf.Ln(10)
			pdf.SetFont("chinese", "", 9)
		}

		// 设置行背景色
		if fill {
			pdf.SetFillColor(249, 249, 249)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}

		// 获取详单类型的中文描述
		detailTypeText := detailTypeMap[detail.DetailType]

		// 绘制单元格内容
		pdf.Cell(25, rowHeight, fmt.Sprintf("%d", bill.RoomID))
		pdf.Cell(35, rowHeight, detail.QueryTime.Format("15:04:05"))
		pdf.Cell(35, rowHeight, detail.StartTime.Format("15:04:05"))
		pdf.Cell(35, rowHeight, detail.EndTime.Format("15:04:05"))
		pdf.Cell(25, rowHeight, fmt.Sprintf("%.1f分钟", detail.ServeTime))
		pdf.Cell(20, rowHeight, detail.Speed)
		pdf.Cell(25, rowHeight, fmt.Sprintf("%.2f元/度", detail.Rate))
		pdf.Cell(25, rowHeight, fmt.Sprintf("%.1f°C", detail.CurrentTemp))
		pdf.Cell(25, rowHeight, fmt.Sprintf("%.1f°C", detail.TargetTemp))

		// 设置费用颜色
		if detail.Cost > 0 {
			pdf.SetTextColor(204, 0, 0)
		}
		pdf.Cell(20, rowHeight, fmt.Sprintf("%.2f元", detail.Cost))
		pdf.SetTextColor(0, 0, 0)

		// 设置操作类型颜色
		switch detail.DetailType {
		case db.DetailTypeServiceStart:
			pdf.SetTextColor(0, 153, 0)
		case db.DetailTypeServiceInterrupt:
			pdf.SetTextColor(204, 0, 0)
		case db.DetailTypeSpeedChange:
			pdf.SetTextColor(0, 102, 204)
		}
		pdf.Cell(30, rowHeight, detailTypeText)
		pdf.SetTextColor(0, 0, 0)

		pdf.Ln(rowHeight)
		fill = !fill
	}
}

func drawFooter(pdf *gofpdf.Fpdf) {
	pdf.SetFont("chinese", "", 8)
	pdf.SetTextColor(128, 128, 128)

	footerText := fmt.Sprintf(
		"打印时间: %s    本详单仅作查询使用，如有疑问请咨询前台",
		time.Now().Format("2006-01-02 15:04:05"),
	)

	footerWidth := pdf.GetStringWidth(footerText)
	pageWidth := 297.0 // A4纸横向宽度
	x := (pageWidth - footerWidth) / 2

	pdf.Text(x, pdf.GetY(), footerText)
}

// GenerateBillPDF 生成账单PDF
func GenerateBillPDF(bill Bill) (*gofpdf.Fpdf, error) {
	// 创建新的PDF文档（使用竖向A4纸）
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// 添加中文字体
	pdf.AddUTF8Font("chinese", "", "./SimHei.ttf")

	// 设置标题
	pdf.SetFont("chinese", "", 20)
	pdf.Cell(190, 15, "住宿账单")
	pdf.Ln(20)

	// 设置基本信息字体
	pdf.SetFont("chinese", "", 12)

	// 添加账单编号和日期
	pdf.Cell(95, 8, fmt.Sprintf("账单编号: B%d%s",
		bill.RoomID,
		time.Now().Format("20060102150405")))
	pdf.Cell(95, 8, fmt.Sprintf("打印日期: %s",
		time.Now().Format("2006-01-02 15:04:05")))
	pdf.Ln(15)

	// 添加分隔线
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(10)

	// 客户信息部分
	pdf.SetFont("chinese", "", 12)
	// 第一行
	pdf.Cell(30, 8, "房间号:")
	pdf.Cell(65, 8, fmt.Sprintf("%d", bill.RoomID))
	pdf.Cell(30, 8, "客户姓名:")
	pdf.Cell(65, 8, bill.ClientName)
	pdf.Ln(10)

	// 第二行
	pdf.Cell(30, 8, "身份证号:")
	pdf.Cell(160, 8, bill.ClientID)
	pdf.Ln(10)

	// 第三行
	pdf.Cell(30, 8, "入住时间:")
	pdf.Cell(160, 8, bill.CheckInTime.Format("2006-01-02 15:04:05"))
	pdf.Ln(10)

	// 第四行
	pdf.Cell(30, 8, "退房时间:")
	pdf.Cell(160, 8, bill.CheckOutTime.Format("2006-01-02 15:04:05"))
	pdf.Ln(10)

	// 添加分隔线
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(10)

	// 费用明细部分
	pdf.SetFont("chinese", "", 12)
	pdf.Cell(190, 10, "费用明细")
	pdf.Ln(12)

	// 设置表格字体
	pdf.SetFont("chinese", "", 11)

	// 住宿费用
	pdf.Cell(95, 8, "住宿天数:")
	pdf.Cell(95, 8, fmt.Sprintf("%d天", bill.DaysStayed))
	pdf.Ln(8)
	pdf.Cell(95, 8, "房间日费率:")
	pdf.Cell(95, 8, fmt.Sprintf("%.2f元/天", bill.RoomRate))
	pdf.Ln(8)
	pdf.Cell(95, 8, "住宿费用小计:")
	pdf.Cell(95, 8, fmt.Sprintf("%.2f元", bill.TotalRoom))
	pdf.Ln(8)

	// 空调费用
	pdf.Cell(95, 8, "空调费用小计:")
	pdf.Cell(95, 8, fmt.Sprintf("%.2f元", bill.TotalAC))
	pdf.Ln(8)

	// 押金
	pdf.Cell(95, 8, "押金:")
	pdf.Cell(95, 8, fmt.Sprintf("%.2f元", bill.Deposit))
	pdf.Ln(15)

	// 添加分隔线
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(10)

	// 总计金额
	pdf.SetFont("chinese", "", 14)
	pdf.Cell(95, 10, "应付总额:")
	pdf.Cell(95, 10, fmt.Sprintf("%.2f元", bill.FinalTotal))
	pdf.Ln(20)

	// 添加备注
	pdf.SetFont("chinese", "", 10)
	pdf.Cell(190, 8, "备注：")
	pdf.Ln(8)
	pdf.Cell(190, 8, "1. 应付总额 = 住宿费用 + 空调费用 - 押金")
	pdf.Ln(8)
	pdf.Cell(190, 8, "2. 如需空调费用详单，请向前台索取")
	pdf.Ln(8)
	pdf.Cell(190, 8, "3. 请保管好此账单，作为缴费凭证")

	// 添加页脚
	pdf.SetY(-15)
	pdf.SetFont("chinese", "", 8)
	pdf.Cell(190, 10, fmt.Sprintf("波普特酒店 - 打印时间: %s",
		time.Now().Format("2006-01-02 15:04:05")))

	return pdf, nil
}
