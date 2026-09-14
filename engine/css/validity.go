// engine/css/validity.go — 表单约束校验（constraint validation）的 CSS 侧接入。
//
// CSS 的 :valid / :invalid / :in-range / :out-of-range 需要知道元素的约束
// 校验状态，而这属于 HTML 表单语义（html5 包的 ValidityState）。css 包不能
// 直接 import html5——html5 已经 import css（engine/html5/defaultcss.go 的 UA 表），
// 反向依赖会形成包循环。因此这里定义一组最简数据结构，由宿主在初始化时
// 注入实现（html5 包的 init 里注册，见 engine/html5/constraint.go 的
// initFormValidityResolver）。
//
// 未注入实现时所有约束校验伪类都不匹配（与「未建模」时的行为一致），
// 因此单独使用 css 包（测试、诊断工具）不需要额外接线。
package css

import "wb-ui/engine/dom"

// FormValidity 是一个表单控件的约束校验结果快照。
type FormValidity struct {
	// WillValidate = 元素是 candidate for constraint validation（未被
	// disabled/readonly/datalist 后代等条件排除）。barred 的元素既非
	// :valid 也非 :invalid。
	WillValidate bool

	// Valid = 没有违反任何约束（含自定义错误）。
	Valid bool

	// RangeLimited = 元素有范围限制（min/max 存在且能按类型解析）。
	// :in-range / :out-of-range 只匹配有范围限制的元素。
	RangeLimited bool

	// OutOfRange = suffering from underflow/overflow（不含 step mismatch）。
	OutOfRange bool

	// UserInteracted = 控件的 user validity 为 true（用户交互过：提交过改变、
	// 或表单被尝试提交）。:user-valid / :user-invalid 要求它为 true——这正是
	// 它们与 :valid / :invalid 的唯一区别。
	UserInteracted bool
}

// FormValidityResolver 计算任意元素的约束校验状态；ok=false 表示元素不参与
// 约束校验（普通元素、output/fieldset 等）。
type FormValidityResolver func(el *dom.Element) (FormValidity, bool)

// formValidityResolver 由宿主注入（初始化期一次，之后只读）。
var formValidityResolver FormValidityResolver

// SetFormValidityResolver 注入约束校验实现。传 nil 清除注入（约束校验伪类
// 恢复为永不匹配）。
func SetFormValidityResolver(f FormValidityResolver) {
	formValidityResolver = f
}

// formValidityOf 查询元素的约束校验状态；未注入实现或元素为空时 ok=false。
func formValidityOf(el *dom.Element) (FormValidity, bool) {
	if el == nil || formValidityResolver == nil {
		return FormValidity{}, false
	}
	return formValidityResolver(el)
}
